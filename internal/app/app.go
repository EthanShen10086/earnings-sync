package app

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/filter"
	thttp "trpc.group/trpc-go/trpc-go/http"

	"github.com/EthanShen10086/earnings-sync/internal/agent"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/httpapi"
	"github.com/EthanShen10086/earnings-sync/internal/infra"
	"github.com/EthanShen10086/earnings-sync/internal/job"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func init() {
	filter.Register("opentelemetry", otelServerFilter, otelClientFilter)
}

func otelServerFilter(ctx context.Context, req interface{}, next filter.ServerHandleFunc) (interface{}, error) {
	ctx, span := otel.Tracer("earnings-sync").Start(ctx, "trpc.request")
	defer span.End()
	rsp, err := next(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return rsp, err
}

func otelClientFilter(ctx context.Context, req, rsp interface{}, next filter.ClientHandleFunc) error {
	ctx, span := otel.Tracer("earnings-sync").Start(ctx, "trpc.client")
	defer span.End()
	err := next(ctx, req, rsp)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// Run starts the tRPC-Go HTTP service.
func Run() error {
	settings := config.LoadSettings()
	ctx := context.Background()
	shutdown, err := observe.SetupTracing(ctx, settings.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer shutdown(context.Background())

	pipe, obs, cleanup, err := Build(settings)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup() }()

	// Optional data-plane (Redis/MQ/objects/audit/search/gateway). Missing
	// backends degrade; never blocks sheets/pg minimum path unless INFRA_STRICT.
	stack, err := infra.Open(ctx, settings)
	if err != nil {
		return err
	}
	defer func() { _ = stack.Close() }()

	s := trpc.NewServer()
	api := httpapi.API{Pipe: pipe, Obs: obs, Facts: sheet.ReaderFrom(pipe.Sheets), Infra: stack}
	thttp.RegisterNoProtocolServiceMux(s.Service("trpc.earnings.sync.HTTP"), api.Handler())
	obs.Logger.Info("earnings-sync listening",
		"http", ":8090",
		"admin", ":8091",
		"storage_mode", settings.StorageMode,
		"infra", stack.Summary(),
	)
	go syncOnStart(pipe, obs)
	return s.Serve()
}

// syncOnStart lets the process itself pull filings. Companies without a spreadsheet are skipped so a missing sheet does not spend a model call.
func syncOnStart(pipe *pipeline.Pipeline, obs *observe.Obs) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for _, company := range pipe.Companies.List() {
		if company.SpreadsheetID == "" {
			obs.Logger.Info("startup sync skipped, no spreadsheet", "ticker", company.Ticker)
			continue
		}
		obs.Logger.Info("startup sync", "ticker", company.Ticker)
		if _, err := pipe.Watch(ctx, company.Ticker, false); err != nil {
			obs.Logger.Error("startup sync failed", "ticker", company.Ticker, "err", err.Error())
		}
	}
}

// Build wires config, storage, the model, Sheets/PG dual-write, and Feishu.
func Build(settings config.Settings) (*pipeline.Pipeline, *observe.Obs, func() error, error) {
	noop := func() error { return nil }
	reg, err := config.Load(settings.CompaniesFile)
	if err != nil {
		return nil, nil, noop, err
	}
	macroReg, err := config.LoadMacro(settings.MacroFile)
	if err != nil {
		return nil, nil, noop, err
	}
	obs := observe.New()
	var model extract.Provider
	var chatter agent.Chatter
	if settings.ModelAPIKey != "" {
		provider := extract.HTTPProvider{
			NameStr: "deepseek",
			BaseURL: settings.ModelBaseURL,
			APIKey:  settings.ModelAPIKey,
			Model:   settings.ModelName,
		}
		model = provider
		chatter = provider
	}
	sheets, cleanup, err := BuildStore(context.Background(), settings)
	if err != nil {
		return nil, nil, noop, err
	}
	// Jobs follow DATABASE_URL when set (cross-process idempotency); else jobs.json.
	store, jobCleanup, err := job.OpenAuto(context.Background(), job.OpenOptions{
		Path:          settings.JobStorePath,
		DatabaseURL:   settings.DatabaseURL,
		MigrationsDir: settings.MigrationsDir,
	})
	if err != nil {
		_ = cleanup()
		return nil, nil, noop, err
	}
	prevCleanup := cleanup
	cleanup = func() error {
		err1 := jobCleanup()
		err2 := prevCleanup()
		if err1 != nil {
			return err1
		}
		return err2
	}
	var sender notify.Sender
	if settings.FeishuWebhook != "" {
		sender = notify.Feishu{Webhook: settings.FeishuWebhook, Secret: settings.FeishuSecret}
	}
	return &pipeline.Pipeline{
		Companies:    reg,
		Macro:        macroReg,
		Jobs:         store,
		Extractor:    extract.Extractor{Model: model},
		Sheets:       sheets,
		Notifier:     sender,
		Chatter:      chatter,
		CredFile:     settings.GoogleCreds,
		ShareWith:    settings.GoogleShareWith,
		Obs:          obs,
		SECUserAgent: settings.SECUserAgent,
		Client:       http.DefaultClient,
	}, obs, cleanup, nil
}
