package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/acquire"
	"github.com/EthanShen10086/earnings-sync/internal/agent"
	"github.com/EthanShen10086/earnings-sync/internal/chart"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/job"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/reconcile"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"
)

// Request is one manual ingest.
type Request struct {
	Ticker   string
	URL      string
	Filename string
	Body     []byte
	DryRun   bool
	Force    bool
	Hint     model.Period
}

// Pipeline runs acquire, extract, reconcile, sheet, charts, takeaway, and notify.
type Pipeline struct {
	Companies    *config.Registry
	Jobs         *job.Store
	Extractor    extract.Extractor
	Sheets       sheet.Store
	Notifier     notify.Sender
	Chatter      agent.Chatter
	CredFile     string
	ShareWith    string
	Obs          *observe.Obs
	SECUserAgent string
	SECBaseURL   string
	Client       *http.Client
	group        singleflight.Group
}

func (p *Pipeline) Ingest(ctx context.Context, req Request) (*model.Job, error) {
	req.Ticker = strings.ToUpper(strings.TrimSpace(req.Ticker))
	company, ok := p.Companies.Get(req.Ticker)
	if !ok {
		return nil, fmt.Errorf("unknown ticker %s", req.Ticker)
	}
	doc, err := p.acquire(ctx, req)
	if err != nil {
		return nil, err
	}
	hash := acquire.Hash(req.Ticker, doc.Body)
	if !req.Force {
		if existing, ok := p.reusable(hash, req.DryRun); ok {
			p.Obs.Logger.Info("filing already stored, model not called", "ticker", req.Ticker, "job_id", existing.ID)
			return existing, nil
		}
	}
	v, err, _ := p.group.Do(hash+boolLabel(req.DryRun), func() (any, error) {
		if !req.Force {
			if existing, ok := p.reusable(hash, req.DryRun); ok {
				return existing, nil
			}
		}
		return p.run(ctx, company, doc, hash, req)
	})
	if err != nil {
		return nil, err
	}
	return v.(*model.Job), nil
}

func (p *Pipeline) acquire(ctx context.Context, req Request) (model.Document, error) {
	ctx, span := p.Obs.Start(ctx, "pipeline.acquire", attribute.String("ticker", req.Ticker))
	defer span.End()
	if req.URL != "" {
		doc, err := acquire.FetchURL(ctx, p.Client, req.URL, p.SECUserAgent)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return doc, err
	}
	doc, err := acquire.ReadFile(req.Filename, req.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return doc, err
}

func (p *Pipeline) runScripted(ctx context.Context, company config.Company, doc model.Document, hash string, req Request) (*model.Job, error) {
	start := time.Now()
	j := p.Jobs.Create(company.Ticker, hash, doc.Locator, doc.Source, req.DryRun)
	ctx, span := p.Obs.Start(ctx, "pipeline.run",
		attribute.String("ticker", company.Ticker),
		attribute.String("doc_hash", hash),
		attribute.Bool("dry_run", req.DryRun),
	)
	defer span.End()
	log := p.Obs.WithContext(ctx)

	var usage extract.TokenUsage
	stage := p.stage(ctx, "extract", func(ctx context.Context) error {
		ex, u, err := p.Extractor.FromDocument(ctx, doc, company, req.Hint)
		usage = u
		if err != nil {
			return err
		}
		if req.Hint.Valid() && !ex.Period.Valid() {
			ex.Period = req.Hint
		}
		ex.Ticker = company.Ticker
		j.Extraction = &ex
		return nil
	})
	j.Stages = append(j.Stages, stage)
	if usage.Total > 0 {
		p.Obs.ModelTokens.Add(float64(usage.Total))
	}
	if stage.Error != "" {
		return p.fail(j, stage.Error, span)
	}

	stage = p.stage(ctx, "reconcile", func(context.Context) error {
		j.Warnings = reconcile.Warnings(*j.Extraction)
		p.Obs.ReconcileDiff.Add(float64(len(j.Warnings)))
		return nil
	})
	j.Stages = append(j.Stages, stage)

	j.Charts = chart.Recommend(*j.Extraction)
	if extra, err := p.Extractor.ChartAdvice(ctx, *j.Extraction); err == nil && len(extra) > 0 {
		j.Charts = append(j.Charts, extra...)
	}

	if !req.DryRun {
		stage = p.stage(ctx, "sheet", func(ctx context.Context) error {
			if company.SpreadsheetID == "" {
				return fmt.Errorf("ticker %s has no spreadsheet_id", company.Ticker)
			}
			if p.Sheets == nil {
				return fmt.Errorf("Google 表格还没接上。先运行 go run ./cmd/doctor")
			}
			begun := time.Now()
			n, err := p.Sheets.Upsert(ctx, company.SpreadsheetID, *j.Extraction)
			p.Obs.SheetLatency.Observe(time.Since(begun).Seconds())
			if err != nil {
				return err
			}
			j.Cells = n
			p.Obs.Cells.Add(float64(n))
			j.SheetURL = company.SheetURL()
			_, err = p.Sheets.ApplyCharts(ctx, company.SpreadsheetID, ruleCharts(j.Charts))
			return err
		})
		j.Stages = append(j.Stages, stage)
		if stage.Error != "" {
			return p.fail(j, stage.Error, span)
		}
	} else {
		j.SheetURL = company.SheetURL()
	}

	stage = p.stage(ctx, "takeaway", func(ctx context.Context) error {
		lines, u, _ := p.Extractor.Takeaway(ctx, *j.Extraction, j.Warnings)
		if u.Total > 0 {
			p.Obs.ModelTokens.Add(float64(u.Total))
		}
		j.Takeaway = lines
		return nil
	})
	j.Stages = append(j.Stages, stage)

	if !req.DryRun && p.Notifier != nil {
		stage = p.stage(ctx, "notify", func(ctx context.Context) error {
			err := p.Notifier.Send(ctx, notify.Message{
				Ticker:   company.Ticker,
				Period:   j.Extraction.Period.Label(),
				Takeaway: j.Takeaway,
				Warnings: j.Warnings,
				Charts:   j.Charts,
				SheetURL: j.SheetURL,
			})
			if err != nil {
				p.Obs.NotifyFail.Inc()
			}
			return err
		})
		j.Stages = append(j.Stages, stage)
		if stage.Error != "" {
			return p.fail(j, stage.Error, span)
		}
		j.Notified = true
	}

	j.Status = model.StatusSucceeded
	if err := p.Jobs.Save(j); err != nil {
		return nil, err
	}
	p.Obs.JobDuration.WithLabelValues(company.Ticker, j.Status, boolLabel(req.DryRun)).Observe(time.Since(start).Seconds())
	log.Info("job finished", "job_id", j.ID, "ticker", company.Ticker, "dry_run", req.DryRun, "cells", j.Cells)
	return j, nil
}

func (p *Pipeline) runPrepared(ctx context.Context, company config.Company, doc model.Document, hash string, req Request, periods []model.Extraction, analysis []string) (*model.Job, error) {
	start := time.Now()
	j := p.Jobs.Create(company.Ticker, hash, doc.Locator, doc.Source, req.DryRun)
	ctx, span := p.Obs.Start(ctx, "pipeline.batch",
		attribute.String("ticker", company.Ticker),
		attribute.String("doc_hash", hash),
		attribute.Int("periods", len(periods)),
		attribute.Bool("dry_run", req.DryRun),
	)
	defer span.End()
	for i := range periods {
		periods[i].Ticker = company.Ticker
		j.Warnings = append(j.Warnings, reconcile.Warnings(periods[i])...)
	}
	latest := periods[len(periods)-1]
	j.Extraction = &latest
	j.Charts = chart.Recommend(latest)
	p.Obs.ReconcileDiff.Add(float64(len(j.Warnings)))

	if !req.DryRun {
		stage := p.stage(ctx, "sheet", func(ctx context.Context) error {
			if company.SpreadsheetID == "" {
				return fmt.Errorf("ticker %s has no spreadsheet_id", company.Ticker)
			}
			if p.Sheets == nil {
				return fmt.Errorf("Google 表格还没接上。先运行 go run ./cmd/doctor")
			}
			begun := time.Now()
			n, err := p.Sheets.UpsertAll(ctx, company.SpreadsheetID, periods)
			p.Obs.SheetLatency.Observe(time.Since(begun).Seconds())
			if err != nil {
				return err
			}
			j.Cells = n
			p.Obs.Cells.Add(float64(n))
			j.SheetURL = company.SheetURL()
			_, err = p.Sheets.ApplyCharts(ctx, company.SpreadsheetID, ruleCharts(j.Charts))
			return err
		})
		j.Stages = append(j.Stages, stage)
		if stage.Error != "" {
			return p.fail(j, stage.Error, span)
		}
	} else {
		j.SheetURL = company.SheetURL()
	}

	stage := p.stage(ctx, "takeaway", func(ctx context.Context) error {
		if len(analysis) > 0 {
			j.Takeaway = analysis
			return nil
		}
		lines, usage, _ := p.Extractor.Takeaway(ctx, latest, j.Warnings)
		if usage.Total > 0 {
			p.Obs.ModelTokens.Add(float64(usage.Total))
		}
		j.Takeaway = lines
		return nil
	})
	j.Stages = append(j.Stages, stage)

	if !req.DryRun && p.Notifier != nil {
		stage = p.stage(ctx, "notify", func(ctx context.Context) error {
			err := p.Notifier.Send(ctx, notify.Message{
				Ticker:   company.Ticker,
				Period:   latest.Period.Label(),
				Takeaway: j.Takeaway,
				Warnings: j.Warnings,
				Charts:   j.Charts,
				SheetURL: j.SheetURL,
			})
			if err != nil {
				p.Obs.NotifyFail.Inc()
			}
			return err
		})
		j.Stages = append(j.Stages, stage)
		if stage.Error != "" {
			return p.fail(j, stage.Error, span)
		}
		j.Notified = true
	}

	j.Status = model.StatusSucceeded
	if err := p.Jobs.Save(j); err != nil {
		return nil, err
	}
	p.Obs.JobDuration.WithLabelValues(company.Ticker, j.Status, boolLabel(req.DryRun)).Observe(time.Since(start).Seconds())
	return j, nil
}

func (p *Pipeline) Watch(ctx context.Context, ticker string, dry bool) ([]*model.Job, error) {
	ticker = strings.ToUpper(ticker)
	var out []*model.Job
	for _, company := range p.Companies.List() {
		if ticker != "" && company.Ticker != ticker {
			continue
		}
		for _, src := range company.Sources {
			if src.Type != "sec_edgar" {
				continue
			}
			filing, ok, err := acquire.LatestFinancials(ctx, p.Client, p.SECBaseURL, src.CIK, p.SECUserAgent, company.FiscalYearEndMonth)
			if err != nil {
				return out, err
			}
			if !ok {
				continue
			}
			if filing.Form == "8-K" {
				if exhibit, err := acquire.ExhibitURL(ctx, p.Client, src.CIK, filing.Accession, p.SECUserAgent); err == nil && exhibit != "" {
					filing.URL = exhibit
				}
				filing.Period = model.Period{}
			}
			if existing, seen := p.Jobs.ByLocator(filing.URL); seen && existing.DryRun == dry {
				out = append(out, existing)
				continue
			}
			job, err := p.Ingest(ctx, Request{Ticker: company.Ticker, URL: filing.URL, DryRun: dry, Hint: filing.Period})
			if err != nil {
				return out, err
			}
			out = append(out, job)
		}
	}
	return out, nil
}

func (p *Pipeline) Recommend(ctx context.Context, jobID string) ([]model.ChartSpec, error) {
	j, err := p.must(jobID)
	if err != nil {
		return nil, err
	}
	if j.Extraction == nil {
		return nil, fmt.Errorf("job has no extraction")
	}
	return j.Charts, nil
}

func (p *Pipeline) Apply(ctx context.Context, jobID, instruction string, rules bool) ([]model.ChartSpec, error) {
	j, err := p.must(jobID)
	if err != nil {
		return nil, err
	}
	if j.Extraction == nil {
		return nil, fmt.Errorf("job has no extraction")
	}
	var specs []model.ChartSpec
	if rules || instruction == "" {
		specs = append(specs, ruleCharts(j.Charts)...)
	}
	if instruction != "" {
		specs = append(specs, chart.FromInstruction(instruction, *j.Extraction)...)
		if len(specs) == 0 {
			return nil, fmt.Errorf("无法从指令判断图表")
		}
	}
	company, _ := p.Companies.Get(j.Ticker)
	if company.SpreadsheetID == "" || p.Sheets == nil {
		return specs, fmt.Errorf("Google 表格还没接上。先运行 go run ./cmd/doctor")
	}
	if _, err := p.Sheets.ApplyCharts(ctx, company.SpreadsheetID, specs); err != nil {
		return nil, err
	}
	return specs, nil
}

func (p *Pipeline) Get(id string) (*model.Job, error) {
	return p.must(id)
}

func (p *Pipeline) must(id string) (*model.Job, error) {
	j, ok := p.Jobs.Get(id)
	if !ok {
		return nil, fmt.Errorf("job not found")
	}
	return j, nil
}

func (p *Pipeline) stage(ctx context.Context, name string, fn func(context.Context) error) model.Stage {
	ctx, span := p.Obs.Start(ctx, "pipeline."+name)
	defer span.End()
	start := time.Now()
	err := fn(ctx)
	st := model.Stage{Name: name, DurationMS: time.Since(start).Milliseconds()}
	if err != nil {
		st.Error = err.Error()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return st
}

func (p *Pipeline) fail(j *model.Job, msg string, span trace.Span) (*model.Job, error) {
	j.Status = model.StatusFailed
	j.Error = msg
	_ = p.Jobs.Save(j)
	err := fmt.Errorf("%s", msg)
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
	p.Obs.JobDuration.WithLabelValues(j.Ticker, j.Status, boolLabel(j.DryRun)).Observe(0)
	return j, nil
}

func ruleCharts(in []model.ChartSpec) []model.ChartSpec {
	var out []model.ChartSpec
	for _, c := range in {
		if c.Source == model.SourceRule {
			out = append(out, c)
		}
	}
	return out
}

func (p *Pipeline) reusable(hash string, dry bool) (*model.Job, bool) {
	existing, ok := p.Jobs.ByHash(hash)
	if !ok || existing.DryRun != dry || existing.Status != model.StatusSucceeded {
		return nil, false
	}
	return existing, true
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
