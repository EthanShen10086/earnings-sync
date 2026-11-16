package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/acquire"
	"github.com/EthanShen10086/earnings-sync/internal/macro"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"go.opentelemetry.io/otel/attribute"
)

func (p *Pipeline) ingestMacro(ctx context.Context, id string, req Request) (*model.Job, error) {
	series, ok := p.Macro.Get(id)
	if !ok {
		return nil, fmt.Errorf("没有这个宏观序列 %s", id)
	}
	req.Ticker = series.ID
	doc, err := p.acquire(ctx, req)
	if err != nil {
		return nil, err
	}
	hash := acquire.Hash(series.ID, doc.Body)
	if !req.Force {
		if existing, ok := p.reusable(hash, req.DryRun); ok {
			p.Obs.Logger.Info("macro file already stored", "series", series.ID, "job_id", existing.ID)
			return existing, nil
		}
	}
	v, err, _ := p.group.Do("macro:"+hash+boolLabel(req.DryRun), func() (any, error) {
		if !req.Force {
			if existing, ok := p.reusable(hash, req.DryRun); ok {
				return existing, nil
			}
		}
		return p.runMacro(ctx, series.ID, doc, hash, req)
	})
	if err != nil {
		return nil, err
	}
	return v.(*model.Job), nil
}

func (p *Pipeline) runMacro(ctx context.Context, id string, doc model.Document, hash string, req Request) (*model.Job, error) {
	series, _ := p.Macro.Get(id)
	start := time.Now()
	j := p.Jobs.Create(series.ID, hash, doc.Locator, doc.Source, req.DryRun)
	j.Kind = model.KindMacro
	ctx, span := p.Obs.Start(ctx, "pipeline.macro",
		attribute.String("series", series.ID),
		attribute.String("doc_hash", hash),
		attribute.Bool("dry_run", req.DryRun),
	)
	defer span.End()

	points, warnings, err := macro.Parse(doc.Text)
	if err != nil {
		return p.fail(j, err.Error(), span)
	}
	rel := model.MacroRelease{Series: series.ID, Title: series.Title, Points: points}
	j.Warnings = append(warnings, macro.Check(points)...)
	j.Macro = &rel
	j.Takeaway = macro.Summary(points)
	latest := macro.LatestPeriod(points)

	if !req.DryRun {
		stage := p.stage(ctx, "sheet", func(ctx context.Context) error {
			if series.SpreadsheetID == "" {
				return fmt.Errorf("宏观序列 %s 还没有 spreadsheet_id。先运行 go run ./cmd/sheetcreate -series %s", series.ID, series.ID)
			}
			if p.Sheets == nil {
				return fmt.Errorf("Google 表格还没接上。先运行 go run ./cmd/doctor")
			}
			begun := time.Now()
			n, err := p.Sheets.UpsertMacro(ctx, series.SpreadsheetID, rel)
			p.Obs.SheetLatency.Observe(time.Since(begun).Seconds())
			if err != nil {
				return err
			}
			j.Cells = n
			p.Obs.Cells.Add(float64(n))
			j.SheetURL = series.SheetURL()
			return nil
		})
		j.Stages = append(j.Stages, stage)
		if stage.Error != "" {
			return p.fail(j, stage.Error, span)
		}
	} else {
		j.SheetURL = series.SheetURL()
	}

	if !req.DryRun && p.Notifier != nil {
		stage := p.stage(ctx, "notify", func(ctx context.Context) error {
			err := p.Notifier.Send(ctx, notify.Message{
				Ticker:   series.Name,
				Period:   latest,
				Kind:     model.KindMacro,
				Takeaway: j.Takeaway,
				Warnings: j.Warnings,
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
	p.Obs.JobDuration.WithLabelValues(series.ID, j.Status, boolLabel(req.DryRun)).Observe(time.Since(start).Seconds())
	return j, nil
}
