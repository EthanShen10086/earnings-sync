package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/job"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

// PipelineBridge exposes pipeline.Ingest as orchestrator tools.
// Sheets may be Google or sheet.Memory; Memory is the offline proof path.
type PipelineBridge struct {
	Pipe   *pipeline.Pipeline
	Memory *sheet.Memory // optional; when set, DumpWrites can export evidence
}

// LocalPipelineOpts configures an offline pipeline that still runs real ingest/update.
type LocalPipelineOpts struct {
	CompaniesFile string
	MacroFile     string
	JobStorePath  string
	Notifier      notify.Sender
	// StorageMode / DatabaseURL opt into PG dual-write. Empty StorageMode keeps Memory-only
	// so unit tests are not affected by process-wide STORAGE_MODE from CI/.env.
	StorageMode   string
	DatabaseURL   string
	MigrationsDir string
}

// NewLocalPipeline builds a Pipeline that parses fixtures and writes through sheet.Memory.
func NewLocalPipeline(opts LocalPipelineOpts) (*PipelineBridge, error) {
	companiesFile := opts.CompaniesFile
	if companiesFile == "" {
		companiesFile = "configs/companies.yaml"
	}
	reg, err := config.Load(companiesFile)
	if err != nil {
		return nil, err
	}
	macroFile := opts.MacroFile
	if macroFile == "" {
		macroFile = "configs/macro.yaml"
	}
	macroReg, err := config.LoadMacro(macroFile)
	if err != nil {
		return nil, err
	}
	jobPath := opts.JobStorePath
	if jobPath == "" {
		jobPath = filepath.Join(os.TempDir(), "earnings-orchestrator-jobs.json")
	}
	if dir := filepath.Dir(jobPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	store, err := job.Open(jobPath)
	if err != nil {
		return nil, err
	}
	mem := &sheet.Memory{}
	sheets := sheet.Store(mem)
	mode := sheet.ParseWriteMode(opts.StorageMode)
	if opts.StorageMode != "" && (mode == sheet.ModeDual || mode == sheet.ModePGOnly) {
		built, cleanup, err := buildStoreLocal(config.Settings{
			StorageMode:   opts.StorageMode,
			DatabaseURL:   opts.DatabaseURL,
			MigrationsDir: opts.MigrationsDir,
			StorageFail:   "all",
		}, mem)
		if err != nil {
			return nil, err
		}
		_ = cleanup // closed by process exit
		sheets = built
	}
	pipe := &pipeline.Pipeline{
		Companies: reg,
		Macro:     macroReg,
		Jobs:      store,
		Sheets:    sheets,
		Notifier:  opts.Notifier,
		Obs:       observe.New(),
	}
	return &PipelineBridge{Pipe: pipe, Memory: mem}, nil
}

// Tools returns tool funcs that call the real pipeline.
func (b *PipelineBridge) Tools() ToolRegistry {
	return ToolRegistry{
		"pipeline_ingest": b.ingest,
	}
}

func (b *PipelineBridge) ingest(ctx context.Context, input any) (any, error) {
	req, err := ingestRequest(input)
	if err != nil {
		return nil, err
	}
	j, err := b.Pipe.Ingest(ctx, req)
	if err != nil {
		return nil, err
	}
	return jobSummary(j), nil
}

func ingestRequest(input any) (pipeline.Request, error) {
	m, _ := input.(map[string]any)
	ticker := strings.ToUpper(strings.TrimSpace(fmt.Sprint(m["ticker"])))
	if ticker == "" || ticker == "<nil>" {
		return pipeline.Request{}, fmt.Errorf("ticker 不能为空")
	}
	req := pipeline.Request{Ticker: ticker}
	if v, ok := m["dry_run"]; ok {
		if dry, ok := asBool(v); ok {
			req.DryRun = dry
		}
	}
	if v, ok := m["force"]; ok {
		if force, ok := asBool(v); ok {
			req.Force = force
		}
	}
	if u := strings.TrimSpace(fmt.Sprint(m["url"])); u != "" && u != "<nil>" {
		req.URL = u
	}
	filename := strings.TrimSpace(fmt.Sprint(m["filename"]))
	if filename == "" || filename == "<nil>" {
		filename = strings.TrimSpace(fmt.Sprint(m["fixture"]))
	}
	bodyPath := strings.TrimSpace(fmt.Sprint(m["body_path"]))
	if bodyPath == "" || bodyPath == "<nil>" {
		bodyPath = strings.TrimSpace(fmt.Sprint(m["path"]))
	}
	if bodyPath != "" && bodyPath != "<nil>" {
		body, err := os.ReadFile(bodyPath)
		if err != nil {
			return pipeline.Request{}, fmt.Errorf("读取 fixture: %w", err)
		}
		req.Body = body
		if filename == "" || filename == "<nil>" {
			filename = filepath.Base(bodyPath)
		}
		req.Filename = filename
	} else if raw, ok := m["body"]; ok {
		switch t := raw.(type) {
		case string:
			req.Body = []byte(t)
		case []byte:
			req.Body = t
		default:
			return pipeline.Request{}, fmt.Errorf("body 类型不支持: %T", raw)
		}
		if filename == "" || filename == "<nil>" {
			filename = ticker + ".csv"
		}
		req.Filename = filename
	}
	if req.URL == "" && len(req.Body) == 0 {
		return pipeline.Request{}, fmt.Errorf("pipeline_ingest 需要 url、body 或 body_path/fixture")
	}
	return req, nil
}

func jobSummary(j *model.Job) map[string]any {
	if j == nil {
		return map[string]any{}
	}
	out := map[string]any{
		"job_id":   j.ID,
		"ticker":   j.Ticker,
		"status":   j.Status,
		"dry_run":  j.DryRun,
		"cells":    j.Cells,
		"notified": j.Notified,
		"warnings": j.Warnings,
		"takeaway": j.Takeaway,
		"locator":  j.Locator,
		"doc_hash": j.DocHash,
		"written":  j.Cells > 0 && !j.DryRun,
	}
	if j.SheetURL != "" {
		out["sheet_url"] = j.SheetURL
	}
	if j.Error != "" {
		out["error"] = j.Error
	}
	if j.Extraction != nil {
		out["period"] = map[string]any{
			"fiscal_year":    j.Extraction.Period.FiscalYear,
			"fiscal_quarter": j.Extraction.Period.FiscalQuarter,
			"label":          j.Extraction.Period.Label(),
		}
		facts := make([]any, 0, len(j.Extraction.Facts))
		for _, f := range j.Extraction.Facts {
			item := map[string]any{"line_item": f.LineItem, "unit": f.Unit}
			if f.Value != nil {
				item["value"] = *f.Value
			}
			if f.Text != "" {
				item["text"] = f.Text
			}
			facts = append(facts, item)
		}
		out["facts"] = facts
	}
	return out
}

// DumpWrites writes Memory sheet cells to path as JSON evidence of a real upsert.
func (b *PipelineBridge) DumpWrites(path string) error {
	if b == nil || b.Memory == nil {
		return fmt.Errorf("没有 Memory sheet，无法导出写入证据")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	payload := map[string]any{
		"writes": b.Memory.Writes,
		"charts": b.Memory.Charts,
		"cells":  len(b.Memory.Writes),
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// MergeTools overlays override entries onto base (override wins).
func MergeTools(base, override ToolRegistry) ToolRegistry {
	out := make(ToolRegistry, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}
