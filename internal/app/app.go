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
	"github.com/EthanShen10086/earnings-sync/internal/auth"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/features"
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
	feat := features.Resolve(features.FromEnv())
	otelEndpoint := settings.OTLPEndpoint
	if !feat.UseOTel() {
		otelEndpoint = ""
	}
	shutdown, err := observe.SetupTracing(ctx, otelEndpoint)
	if err != nil {
		return err
	}
	defer shutdown(context.Background())

	pipe, obs, storeBuild, cleanup, err := BuildWithFeatures(settings, feat)
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

	authCfg := auth.LoadConfig()
	s := trpc.NewServer()
	api := httpapi.API{
		Pipe:   pipe,
		Obs:    obs,
		Facts:  sheet.ReaderFrom(pipe.Sheets),
		Auth:   authCfg,
		Alerts: observe.NewAlertBoard(),
		Infra:  stack,
		Feat:   feat,
		Storage: httpapi.StorageInfo{
			WantMode:   string(storeBuild.WantMode),
			Mode:       string(storeBuild.Mode),
			FailPolicy: settings.StorageFail,
			SheetsKind: storeBuild.SheetsKind,
			PGKind:     storeBuild.PGKind,
			Notes:      storeBuild.Notes,
		},
	}
	thttp.RegisterNoProtocolServiceMux(s.Service("trpc.earnings.sync.HTTP"), api.Handler())
	obs.Logger.Info("earnings-sync listening",
		"http", ":8090",
		"admin", ":8091",
		"storage_mode", settings.StorageMode,
		"infra", stack.Summary(),
		"auth_mode", authCfg.Mode,
	)
	for _, st := range feat.Report {
		obs.Logger.Info("feature", "name", st.Name, "want", string(st.Want), "state", string(st.State), "detail", st.Detail)
	}
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
func Build(settings config.Settings) (*pipeline.Pipeline, *observe.Obs, StoreBuild, func() error, error) {
	return BuildWithFeatures(settings, features.Resolve(features.FromEnv()))
}

// BuildWithFeatures is Build with an explicit feature bundle (tests / CLI).
func BuildWithFeatures(settings config.Settings, feat features.Bundle) (*pipeline.Pipeline, *observe.Obs, StoreBuild, func() error, error) {
	noop := func() error { return nil }
	empty := StoreBuild{Cleanup: noop}
	reg, err := config.Load(settings.CompaniesFile)
	if err != nil {
		return nil, nil, empty, noop, err
	}
	macroReg, err := config.LoadMacro(settings.MacroFile)
	if err != nil {
		return nil, nil, empty, noop, err
	}
	obs := observe.New()
	var model extract.Provider
	var chatter agent.Chatter
	if feat.UseLLM() {
		if feat.LiveLLM() && settings.ModelAPIKey != "" {
			provider := extract.HTTPProvider{
				NameStr: "deepseek",
				BaseURL: settings.ModelBaseURL,
				APIKey:  settings.ModelAPIKey,
				Model:   settings.ModelName,
			}
			model = provider
			chatter = provider
		} else {
			model = extract.StubProvider{}
			chatter = &extract.StubChatter{}
			obs.Logger.Info("llm stub provider active", "reason", "FEATURE_LLM on without DEEPSEEK_API_KEY")
		}
	}
	built, err := BuildStoreDetailed(context.Background(), settings, feat)
	if err != nil {
		return nil, nil, empty, noop, err
	}
	cleanup := built.Cleanup
	for _, note := range built.Notes {
		obs.Logger.Info("storage degrade", "note", note, "mode", string(built.Mode), "sheets", built.SheetsKind, "pg", built.PGKind)
	}
	// Jobs follow DATABASE_URL when set (cross-process idempotency); else jobs.json.
	// If PG was requested for facts but unavailable, still try jobs file fallback.
	dbURL := settings.DatabaseURL
	if built.PGKind != "postgres" {
		dbURL = ""
	}
	store, jobCleanup, err := job.OpenAuto(context.Background(), job.OpenOptions{
		Path:          settings.JobStorePath,
		DatabaseURL:   dbURL,
		MigrationsDir: settings.MigrationsDir,
	})
	if err != nil {
		_ = cleanup()
		return nil, nil, built, noop, err
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
	if feat.UseNotify() {
		if feat.LiveNotify() && settings.FeishuWebhook != "" {
			sender = notify.Feishu{Webhook: settings.FeishuWebhook, Secret: settings.FeishuSecret}
		} else {
			sender = &notify.Memory{}
			obs.Logger.Info("notify stub active", "reason", "FEATURE_NOTIFY on without FEISHU_WEBHOOK_URL")
		}
	}
	return &pipeline.Pipeline{
		Companies:    reg,
		Macro:        macroReg,
		Jobs:         store,
		Extractor:    extract.Extractor{Model: model},
		Sheets:       built.Store,
		Notifier:     sender,
		Chatter:      chatter,
		CredFile:     settings.GoogleCreds,
		ShareWith:    settings.GoogleShareWith,
		Obs:          obs,
		SECUserAgent: settings.SECUserAgent,
		Client:       http.DefaultClient,
	}, obs, built, cleanup, nil
}
