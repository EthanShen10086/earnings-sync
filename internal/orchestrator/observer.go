package orchestrator

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// ProductEvent is a minimal product-loop signal: start / success / fail (+ step timing).
type ProductEvent struct {
	Name       string         `json:"name"` // flow.start | flow.success | flow.fail | step.success | step.fail
	At         time.Time      `json:"at"`
	RunID      string         `json:"run_id"`
	RequestID  string         `json:"request_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	NodeID     string         `json:"node_id,omitempty"`
	Status     Status         `json:"status,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
	Error      string         `json:"error,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
}

// Observer hooks structured logs, product events, and optional metrics.
// Implementations must be safe for concurrent use from the engine pump.
type Observer interface {
	Log(ctx context.Context, level slog.Level, msg string, attrs ...any)
	Event(ctx context.Context, ev ProductEvent)
	NodeDuration(nodeType string, status Status, d time.Duration)
	RunDuration(status Status, d time.Duration)
}

// SlogObserver writes JSON logs to stderr and forwards product events to a sink.
type SlogObserver struct {
	Logger *slog.Logger
	Sink   EventSink // optional; nil discards product events
}

// EventSink persists or forwards product-loop events.
type EventSink interface {
	Append(ev ProductEvent) error
}

// NewSlogObserver returns a JSON logger on stderr.
func NewSlogObserver(sink EventSink) *SlogObserver {
	return &SlogObserver{
		Logger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		Sink:   sink,
	}
}

func (o *SlogObserver) Log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if o == nil || o.Logger == nil {
		return
	}
	attrs = append(attrs,
		"request_id", RequestIDFrom(ctx),
		"trace_id", TraceIDFrom(ctx),
		"run_id", RunIDFrom(ctx),
	)
	o.Logger.Log(ctx, level, msg, attrs...)
}

func (o *SlogObserver) Event(ctx context.Context, ev ProductEvent) {
	if o == nil {
		return
	}
	if ev.RequestID == "" {
		ev.RequestID = RequestIDFrom(ctx)
	}
	if ev.TraceID == "" {
		ev.TraceID = TraceIDFrom(ctx)
	}
	if ev.RunID == "" {
		ev.RunID = RunIDFrom(ctx)
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	o.Log(ctx, slog.LevelInfo, "product_event",
		"event", ev.Name,
		"node_id", ev.NodeID,
		"status", string(ev.Status),
		"duration_ms", ev.DurationMs,
		"error", ev.Error,
	)
	if o.Sink != nil {
		_ = o.Sink.Append(ev)
	}
}

func (o *SlogObserver) NodeDuration(string, Status, time.Duration) {}
func (o *SlogObserver) RunDuration(Status, time.Duration)          {}

// MetricsObserver decorates another Observer with Prometheus histograms from observe.Obs.
type MetricsObserver struct {
	Inner Observer
	Rec   MetricsRecorder
}

// MetricsRecorder is the subset of observe.Obs used by the orchestrator.
type MetricsRecorder interface {
	ObserveFlowRun(status string, seconds float64)
	ObserveFlowNode(nodeType, status string, seconds float64)
}

func (o *MetricsObserver) Log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if o.Inner != nil {
		o.Inner.Log(ctx, level, msg, attrs...)
	}
}

func (o *MetricsObserver) Event(ctx context.Context, ev ProductEvent) {
	if o.Inner != nil {
		o.Inner.Event(ctx, ev)
	}
}

func (o *MetricsObserver) NodeDuration(nodeType string, status Status, d time.Duration) {
	if o.Rec != nil {
		o.Rec.ObserveFlowNode(nodeType, string(status), d.Seconds())
	}
	if o.Inner != nil {
		o.Inner.NodeDuration(nodeType, status, d)
	}
}

func (o *MetricsObserver) RunDuration(status Status, d time.Duration) {
	if o.Rec != nil {
		o.Rec.ObserveFlowRun(string(status), d.Seconds())
	}
	if o.Inner != nil {
		o.Inner.RunDuration(status, d)
	}
}
