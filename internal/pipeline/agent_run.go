package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/acquire"
	"github.com/EthanShen10086/earnings-sync/internal/agent"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Chatter lets DeepSeek call sheet and notification tools. When it is set,
// ingest is an agent loop instead of a fixed extract-then-write script.
func (p *Pipeline) run(ctx context.Context, company config.Company, doc model.Document, hash string, req Request) (*model.Job, error) {
	if periods, ok := extract.TryStructuredAll(doc.Text, doc.Filename); ok {
		return p.runPrepared(ctx, company, doc, hash, req, periods, nil)
	}
	if p.Extractor.Model != nil {
		return p.runFromReport(ctx, company, doc, hash, req)
	}
	if p.Chatter != nil {
		return p.runAgent(ctx, company, doc, hash, req)
	}
	return p.runScripted(ctx, company, doc, hash, req)
}

func (p *Pipeline) runAgent(ctx context.Context, company config.Company, doc model.Document, hash string, req Request) (*model.Job, error) {
	start := time.Now()
	j := p.Jobs.Create(company.Ticker, hash, doc.Locator, doc.Source, req.DryRun)
	ctx, span := p.Obs.Start(ctx, "pipeline.agent",
		attribute.String("ticker", company.Ticker),
		attribute.String("doc_hash", hash),
		attribute.Bool("dry_run", req.DryRun),
	)
	defer span.End()
	text := doc.Text
	if len(text) > 50000 {
		text = text[:50000]
	}
	mode := "可以写入表格、新建表格、生成图表并发送飞书。"
	if req.DryRun {
		mode = "这是 dry_run。工具会被调用，但不会真正写表或发消息。"
	}
	user := fmt.Sprintf("公司 %s（%s），财年截止月 %d。已有表格：%t。期间提示：%s。%s\n\n%s",
		company.Name, company.Ticker, company.FiscalYearEndMonth, company.SpreadsheetID != "", req.Hint.Label(), mode, text)
	stage := p.stage(ctx, "agent", func(ctx context.Context) error {
		result, err := agent.Run(ctx, p.Chatter, agent.Runtime{
			Companies: p.Companies,
			Sheets:    p.Sheets,
			Notifier:  p.Notifier,
			CredFile:  p.CredFile,
			ShareWith: p.ShareWith,
			DryRun:    req.DryRun,
		}, user)
		if result.Tokens > 0 {
			p.Obs.ModelTokens.Add(float64(result.Tokens))
		}
		j.Actions = result.Actions
		j.Takeaway = result.Takeaway
		j.Warnings = result.Warnings
		j.Charts = result.Charts
		j.Cells = result.Cells
		j.Notified = result.Notified
		j.SheetURL = result.SheetURL
		j.Extraction = result.Extraction
		if j.SheetURL == "" {
			j.SheetURL = company.SheetURL()
		}
		return err
	})
	j.Stages = append(j.Stages, stage)
	if stage.Error != "" {
		return p.fail(j, stage.Error, span)
	}
	if len(j.Actions) == 0 {
		return p.fail(j, "模型没有调用任何工具。deepseek-chat 不能当作执行模型，请把 DEEPSEEK_MODEL 设为 deepseek-flash 或 deepseek-v4-pro", span)
	}
	j.Status = model.StatusSucceeded
	if err := p.Jobs.Save(j); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	p.Obs.JobDuration.WithLabelValues(company.Ticker, j.Status, boolLabel(req.DryRun)).Observe(time.Since(start).Seconds())
	p.Obs.WithContext(ctx).Info("agent finished", "job_id", j.ID, "actions", strings.Join(j.Actions, ","))
	return j, nil
}

func (p *Pipeline) runFromReport(ctx context.Context, company config.Company, doc model.Document, hash string, req Request) (*model.Job, error) {
	ctx, span := p.Obs.Start(ctx, "pipeline.parse",
		attribute.String("ticker", company.Ticker),
		attribute.String("doc_hash", hash),
	)
	defer span.End()
	periods, analysis, usage, err := p.Extractor.ParseReport(ctx, doc, company, req.Hint)
	if usage.Total > 0 {
		p.Obs.ModelTokens.Add(float64(usage.Total))
	}
	if err != nil {
		j := p.Jobs.Create(company.Ticker, hash, doc.Locator, doc.Source, req.DryRun)
		return p.fail(j, err.Error(), span)
	}
	latest := &periods[len(periods)-1]
	if p.SECBaseURL == "" {
		for _, src := range company.Sources {
			if src.Type == "sec_edgar" && src.CIK != "" {
				_ = acquire.SupplementCash(ctx, p.Client, src.CIK, p.SECUserAgent, company.FiscalYearEndMonth, latest)
				break
			}
		}
	}
	if miss := extract.MissingCore(*latest); len(miss) > 0 {
		j := p.Jobs.Create(company.Ticker, hash, doc.Locator, doc.Source, req.DryRun)
		j.Tokens = usage.Total
		return p.fail(j, fmt.Sprintf("财报解析缺少资产负债表或利润表字段：%s", strings.Join(miss, ", ")), span)
	}
	job, err := p.runPrepared(ctx, company, doc, hash, req, periods, analysis)
	if job != nil && usage.Total > 0 {
		job.Tokens = usage.Total
		_ = p.Jobs.Save(job)
		p.Obs.Logger.Info("deepseek parsed filing", "ticker", company.Ticker, "model_tokens", usage.Total)
	}
	return job, err
}
