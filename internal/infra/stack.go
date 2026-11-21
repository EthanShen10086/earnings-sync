// Package infra wires optional data-plane adapters without blocking the
// minimum ingest path when advanced backends are missing.
package infra

import (
	"context"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/adapters/audit"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/cache"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/gateway"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/mq"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/objectstore"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/search"
	"github.com/EthanShen10086/earnings-sync/internal/config"
)

// Stack holds optional infra clients. Nil fields are never expected after Open;
// Open always returns usable stubs.
type Stack struct {
	Cache   cache.Cache
	Queue   mq.Queue
	Objects objectstore.Store
	Audit   audit.Logger
	Search  search.Index
	Gateway *gateway.Limiter

	Status Status
}

// Status is a JSON-friendly health snapshot for /healthz or doctor.
type Status struct {
	Cache   cache.Status       `json:"cache"`
	Queue   mq.Status          `json:"queue"`
	Objects objectstore.Status `json:"objects"`
	Audit   audit.Status       `json:"audit"`
	Search  search.Status      `json:"search"`
	Gateway gateway.Status     `json:"gateway"`
}

// Open builds the stack. With INFRA_STRICT=false (default), dial failures
// degrade to memory/file stubs and return nil error.
func Open(ctx context.Context, settings config.Settings) (*Stack, error) {
	strict := settings.InfraStrict

	c, cs, err := cache.Open(ctx, settings.RedisURL, strict)
	if err != nil {
		return nil, err
	}
	q, qs, err := mq.Open(ctx, settings.RedisURL, settings.NATSURL, strict)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	o, osStatus, err := objectstore.Open(settings.ObjectStoreDir, settings.CDNBaseURL)
	if err != nil {
		_ = c.Close()
		_ = q.Close()
		return nil, err
	}
	a, as, err := audit.Open(ctx, settings.AuditLogPath, settings.DatabaseURL, strict)
	if err != nil {
		_ = c.Close()
		_ = q.Close()
		return nil, err
	}
	s, ss, err := search.Open(ctx, settings.SearchIndexPath, settings.DatabaseURL, strict)
	if err != nil {
		_ = c.Close()
		_ = q.Close()
		_ = a.Close()
		return nil, err
	}

	lim := gateway.NewLimiter(settings.RateLimitRPS, settings.RateLimitBurst)
	gs := gateway.Status{
		Enabled: settings.RateLimitRPS > 0,
		RPS:     settings.RateLimitRPS,
		Burst:   settings.RateLimitBurst,
		Detail:  "in-process token-bucket; counts allowed/rejected (edge gateway still optional)",
	}
	if !gs.Enabled {
		gs.Detail = "RATE_LIMIT_RPS unset or 0 — pass-through"
	}

	return &Stack{
		Cache:   c,
		Queue:   q,
		Objects: o,
		Audit:   a,
		Search:  s,
		Gateway: lim,
		Status: Status{
			Cache:   cs,
			Queue:   qs,
			Objects: osStatus,
			Audit:   as,
			Search:  ss,
			Gateway: gs,
		},
	}, nil
}

// Close releases backends. Safe on partially opened stacks.
func (s *Stack) Close() error {
	if s == nil {
		return nil
	}
	var first error
	for _, closer := range []func() error{
		s.Cache.Close,
		s.Queue.Close,
		s.Audit.Close,
		s.Search.Close,
	} {
		if closer == nil {
			continue
		}
		if err := closer(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Degraded reports whether any optional backend fell back.
func (s *Stack) Degraded() bool {
	if s == nil {
		return false
	}
	return s.Status.Cache.Degraded ||
		s.Status.Queue.Degraded ||
		s.Status.Objects.Degraded ||
		s.Status.Audit.Degraded ||
		s.Status.Search.Degraded
}

// Summary is a short slog-friendly string.
func (s *Stack) Summary() string {
	if s == nil {
		return "infra=nil"
	}
	parts := []string{
		"cache=" + s.Status.Cache.Backend,
		"mq=" + s.Status.Queue.Backend,
		"objects=" + s.Status.Objects.Backend,
		"audit=" + s.Status.Audit.Backend,
		"search=" + s.Status.Search.Backend,
	}
	if s.Degraded() {
		parts = append(parts, "degraded=true")
	}
	return strings.Join(parts, " ")
}
