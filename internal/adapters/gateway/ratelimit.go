// Package gateway is an API gateway skeleton (rate limit + wrap).
package gateway

import (
	"net/http"
	"sync"
	"time"
)

// Limiter is a simple token-bucket rate limiter keyed by client IP.
type Limiter struct {
	rps   float64
	burst int
	mu    sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewLimiter returns a limiter. rps <= 0 disables limiting (pass-through).
func NewLimiter(rps float64, burst int) *Limiter {
	if burst < 1 {
		burst = 1
	}
	return &Limiter{rps: rps, burst: burst, buckets: map[string]*bucket{}}
}

// Allow reports whether the key may proceed.
func (l *Limiter) Allow(key string) bool {
	if l == nil || l.rps <= 0 {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: float64(l.burst) - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.rps
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware rejects excess traffic with 429. Disabled when rps <= 0.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	if l == nil || l.rps <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			key = xff
		}
		if !l.Allow(key) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Status describes gateway skeleton state.
type Status struct {
	Enabled bool    `json:"enabled"`
	RPS     float64 `json:"rps"`
	Burst   int     `json:"burst"`
	Detail  string  `json:"detail,omitempty"`
}
