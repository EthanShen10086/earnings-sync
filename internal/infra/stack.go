// Package infra wires optional data-plane adapters without blocking the
// minimum ingest path when advanced backends are missing.
package infra

import (
	"context"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/adapters/audit"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/cache"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/gateway"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/kafka"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/mq"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/objectstore"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/search"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/discovery"
	"github.com/EthanShen10086/earnings-sync/internal/observability"
)

// Stack holds optional infra clients. Nil fields are never expected after Open;
// Open always returns usable stubs.
type Stack struct {
	Cache      cache.Cache
	Queue      mq.Queue
	Objects    objectstore.Store
	Audit      audit.Logger
	Search     search.Index
	Gateway    *gateway.Gateway
	Discovery  discovery.Registry
	Kafka      kafka.Bus
	Shipper    *observability.Shipper
	AlertRules []observability.AlertRule

	Status Status
}

// Status is a JSON-friendly health snapshot for /healthz or doctor.
type Status struct {
	Cache      cache.Status                `json:"cache"`
	Queue      mq.Status                   `json:"queue"`
	Objects    objectstore.Status          `json:"objects"`
	Audit      audit.Status                `json:"audit"`
	Search     search.Status               `json:"search"`
	Gateway    gateway.Status              `json:"gateway"`
	Discovery  discovery.Status            `json:"discovery"`
	Kafka      kafka.Status                `json:"kafka"`
	Shipper    observability.ShipperStatus `json:"shipper"`
	AlertRules int                         `json:"alert_rules"`
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

	disc, ds, err := discovery.Open(ctx, settings.DiscoveryBackend, settings.DiscoveryURL, strict)
	if err != nil {
		_ = c.Close()
		_ = q.Close()
		_ = a.Close()
		_ = s.Close()
		return nil, err
	}
	// Self-register the HTTP API when discovery is available.
	_ = disc.Register(ctx, discovery.Instance{
		Service: "earnings-sync",
		ID:      "local-http",
		Host:    "127.0.0.1",
		Port:    8090,
		Meta:    map[string]string{"role": "api"},
	})

	kb, ks, err := kafka.Open(ctx, settings.KafkaBrokers, settings.KafkaEventLog, strict)
	if err != nil {
		_ = c.Close()
		_ = q.Close()
		_ = a.Close()
		_ = s.Close()
		_ = disc.Close()
		return nil, err
	}

	ship, shipSt, err := observability.OpenShipper(settings.LogShipperPath, settings.ElasticsearchURL, "earnings-sync", "earnings-sync")
	if err != nil {
		_ = c.Close()
		_ = q.Close()
		_ = a.Close()
		_ = s.Close()
		_ = disc.Close()
		_ = kb.Close()
		return nil, err
	}

	gw := gateway.New(settings.RateLimitRPS, settings.RateLimitBurst)
	gw.RegisterRoute(gateway.Route{Prefix: "/v1", Service: "earnings-sync"})
	gs := gateway.Status{
		Enabled: settings.RateLimitRPS > 0,
		RPS:     settings.RateLimitRPS,
		Burst:   settings.RateLimitBurst,
		Detail:  "in-process gateway: token-bucket + route table; edge proxy optional",
	}
	if !gs.Enabled {
		gs.Detail = "RATE_LIMIT_RPS unset or 0 — pass-through gateway with route table"
	}

	var rules []observability.AlertRule
	if settings.AlertRulesPath != "" {
		if loaded, loadErr := observability.LoadAlertRules(settings.AlertRulesPath); loadErr == nil {
			rules = loaded
		}
	}

	return &Stack{
		Cache:      c,
		Queue:      q,
		Objects:    o,
		Audit:      a,
		Search:     s,
		Gateway:    gw,
		Discovery:  disc,
		Kafka:      kb,
		Shipper:    ship,
		AlertRules: rules,
		Status: Status{
			Cache:      cs,
			Queue:      qs,
			Objects:    osStatus,
			Audit:      as,
			Search:     ss,
			Gateway:    gs,
			Discovery:  ds,
			Kafka:      ks,
			Shipper:    shipSt,
			AlertRules: len(rules),
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
		func() error {
			if s.Discovery != nil {
				return s.Discovery.Close()
			}
			return nil
		},
		func() error {
			if s.Kafka != nil {
				return s.Kafka.Close()
			}
			return nil
		},
		func() error {
			if s.Shipper != nil {
				return s.Shipper.Close()
			}
			return nil
		},
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
		s.Status.Search.Degraded ||
		s.Status.Discovery.Degraded ||
		s.Status.Kafka.Degraded ||
		s.Status.Shipper.Degraded
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
		"discovery=" + s.Status.Discovery.Backend,
		"kafka=" + s.Status.Kafka.Backend,
		"shipper=" + s.Status.Shipper.Backend,
	}
	if s.Degraded() {
		parts = append(parts, "degraded=true")
	}
	return strings.Join(parts, " ")
}
