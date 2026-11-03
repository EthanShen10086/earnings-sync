package app

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/filter"
	thttp "trpc.group/trpc-go/trpc-go/http"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/httpapi"
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

	pipe, obs, err := Build(settings)
	if err != nil {
		return err
	}
	s := trpc.NewServer()
	api := httpapi.API{Pipe: pipe, Obs: obs}
	thttp.RegisterNoProtocolServiceMux(s.Service("trpc.earnings.sync.HTTP"), api.Handler())
	obs.Logger.Info("earnings-sync listening", "http", ":8090", "admin", ":8091")
	return s.Serve()
}

// Build wires config, storage, the model, Sheets, and Feishu.
func Build(settings config.Settings) (*pipeline.Pipeline, *observe.Obs, error) {
	reg, err := config.Load(settings.CompaniesFile)
	if err != nil {
		return nil, nil, err
	}
	store, err := job.Open(settings.JobStorePath)
	if err != nil {
		return nil, nil, err
	}
	obs := observe.New()
	var model extract.Provider
	if settings.ModelAPIKey != "" {
		model = extract.HTTPProvider{
			NameStr: "deepseek",
			BaseURL: settings.ModelBaseURL,
			APIKey:  settings.ModelAPIKey,
			Model:   settings.ModelName,
		}
	}
	var sheets sheet.Store
	if settings.GoogleCreds != "" {
		g, err := sheet.NewGoogle(context.Background(), settings.GoogleCreds)
		if err != nil {
			return nil, nil, fmt.Errorf("google sheets: %w", err)
		}
		sheets = g
	}
	var sender notify.Sender
	if settings.FeishuWebhook != "" {
		sender = notify.Feishu{Webhook: settings.FeishuWebhook}
	}
	return &pipeline.Pipeline{
		Companies:    reg,
		Jobs:         store,
		Extractor:    extract.Extractor{Model: model},
		Sheets:       sheets,
		Notifier:     sender,
		Obs:          obs,
		SECUserAgent: settings.SECUserAgent,
		Client:       http.DefaultClient,
	}, obs, nil
}
