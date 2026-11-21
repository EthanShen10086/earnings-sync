// Package cache is a pluggable key/value cache (Redis or in-memory degrade).
package cache

import (
	"context"
	"time"
)

// Cache stores short-lived values. Backends may be Redis or process memory.
type Cache interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Ping(ctx context.Context) error
	Close() error
	Backend() string
}

// Status describes how the cache was wired.
type Status struct {
	Backend   string `json:"backend"`
	Degraded  bool   `json:"degraded"`
	Configured bool  `json:"configured"`
	Detail    string `json:"detail,omitempty"`
}
