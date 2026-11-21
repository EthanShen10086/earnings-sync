package observe

import (
	"sync"
	"time"
)

// Alert is a local stub for dashboard / ops wiring. Not a production pager.
type Alert struct {
	ID        string    `json:"id"`
	Severity  string    `json:"severity"` // info | warn | critical
	Title     string    `json:"title"`
	Detail    string    `json:"detail,omitempty"`
	Source    string    `json:"source"` // stub | prometheus | otel
	Firing    bool      `json:"firing"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AlertBoard holds in-process alert placeholders for demos without Grafana.
type AlertBoard struct {
	mu   sync.RWMutex
	list []Alert
}

// NewAlertBoard seeds a few demo rows so /v1/ops/alerts is never empty locally.
func NewAlertBoard() *AlertBoard {
	now := time.Now().UTC()
	return &AlertBoard{list: []Alert{
		{
			ID:        "demo-ingest-latency",
			Severity:  "info",
			Title:     "Ingest p95 within SLO (stub)",
			Detail:    "Placeholder: wire Prometheus rule earnings_job_duration_seconds",
			Source:    "stub",
			Firing:    false,
			UpdatedAt: now,
		},
		{
			ID:        "demo-notify-fail",
			Severity:  "warn",
			Title:     "Feishu notify failures (stub, not firing)",
			Detail:    "Placeholder: alert on earnings_notify_failures_total rate",
			Source:    "stub",
			Firing:    false,
			UpdatedAt: now,
		},
		{
			ID:        "demo-otel-export",
			Severity:  "info",
			Title:     "OTLP endpoint optional",
			Detail:    "Empty OTEL_EXPORTER_OTLP_ENDPOINT keeps noop tracer; Jaeger profile is optional",
			Source:    "otel",
			Firing:    false,
			UpdatedAt: now,
		},
	}}
}

// List returns a copy of current alerts.
func (b *AlertBoard) List() []Alert {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Alert, len(b.list))
	copy(out, b.list)
	return out
}

// Upsert replaces or appends an alert by ID (tests / local stubs).
func (b *AlertBoard) Upsert(a Alert) {
	if b == nil {
		return
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.list {
		if b.list[i].ID == a.ID {
			b.list[i] = a
			return
		}
	}
	b.list = append(b.list, a)
}
