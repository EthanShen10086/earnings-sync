package observe

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Obs holds the process-wide tracer, metrics, and logger.
type Obs struct {
	Registry      *prometheus.Registry
	Tracer        trace.Tracer
	Logger        *slog.Logger
	JobDuration   *prometheus.HistogramVec
	ModelTokens   prometheus.Counter
	SheetLatency  prometheus.Histogram
	ReconcileDiff prometheus.Counter
	NotifyFail    prometheus.Counter
	Cells         prometheus.Counter
}

func New() *Obs {
	reg := prometheus.NewRegistry()
	o := &Obs{
		Registry: reg,
		Tracer:   otel.Tracer("earnings-sync"),
		Logger:   slog.New(slog.NewJSONHandler(os.Stdout, nil)),
		JobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "earnings_job_duration_seconds",
			Help:    "End-to-end ingest job duration.",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 15, 30, 60, 120},
		}, []string{"ticker", "status", "dry_run"}),
		ModelTokens: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "earnings_model_tokens_total",
			Help: "Tokens reported by the model provider.",
		}),
		SheetLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "earnings_sheet_latency_seconds",
			Help:    "Google Sheets write latency.",
			Buckets: prometheus.DefBuckets,
		}),
		ReconcileDiff: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "earnings_reconcile_warnings_total",
			Help: "Reconcile warnings produced.",
		}),
		NotifyFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "earnings_notify_failures_total",
			Help: "Feishu notification failures.",
		}),
		Cells: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "earnings_sheet_cells_written_total",
			Help: "Input cells written to Google Sheets.",
		}),
	}
	reg.MustRegister(o.JobDuration, o.ModelTokens, o.SheetLatency, o.ReconcileDiff, o.NotifyFail, o.Cells)
	return o
}

func (o *Obs) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(o.Registry, promhttp.HandlerOpts{})
}

func (o *Obs) WithContext(ctx context.Context) *slog.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return o.Logger
	}
	return o.Logger.With("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
}

func (o *Obs) Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return o.Tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// SetupTracing installs a global tracer provider. An empty endpoint keeps the default noop provider.
func SetupTracing(ctx context.Context, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName("earnings-sync"),
	))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1))),
	)
	otel.SetTracerProvider(tp)
	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(shutdownCtx)
	}, nil
}
