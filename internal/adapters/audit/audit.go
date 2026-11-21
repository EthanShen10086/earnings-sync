// Package audit records security-relevant write actions (file or PG stub).
package audit

import (
	"context"
	"time"
)

// Event is one audit record.
type Event struct {
	At     time.Time         `json:"at"`
	Actor  string            `json:"actor"`
	Action string            `json:"action"`
	Target string            `json:"target"`
	Meta   map[string]string `json:"meta,omitempty"`
}

// Logger append-only audit trail.
type Logger interface {
	Append(ctx context.Context, e Event) error
	List(ctx context.Context, limit int) ([]Event, error)
	Backend() string
	Close() error
}

// Status describes audit wiring.
type Status struct {
	Backend    string `json:"backend"`
	Degraded   bool   `json:"degraded"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
}
