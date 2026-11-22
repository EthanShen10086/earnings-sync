package gateway

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

// Route maps a path prefix to an upstream URL (or empty for local-only).
type Route struct {
	Prefix   string `json:"prefix"`
	Upstream string `json:"upstream,omitempty"`
	Service  string `json:"service,omitempty"`
}

// Gateway combines rate limiting with an optional path→upstream route table.
// Without upstreams it still records routes and passes through to the next handler
// (in-process API gateway stub).
type Gateway struct {
	*Limiter
	mu     sync.RWMutex
	routes []Route
}

// New builds a gateway with rate limit settings.
func New(rps float64, burst int) *Gateway {
	return &Gateway{Limiter: NewLimiter(rps, burst)}
}

// RegisterRoute adds or replaces a prefix route.
func (g *Gateway) RegisterRoute(r Route) {
	if g == nil || r.Prefix == "" {
		return
	}
	if !strings.HasPrefix(r.Prefix, "/") {
		r.Prefix = "/" + r.Prefix
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.routes {
		if g.routes[i].Prefix == r.Prefix {
			g.routes[i] = r
			return
		}
	}
	g.routes = append(g.routes, r)
}

// Routes returns a copy of registered routes.
func (g *Gateway) Routes() []Route {
	if g == nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]Route, len(g.routes))
	copy(out, g.routes)
	return out
}

// Handler wraps next with rate limit + optional reverse proxy for registered prefixes.
func (g *Gateway) Handler(next http.Handler) http.Handler {
	if g == nil {
		return next
	}
	base := g.Limiter.Middleware(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if route, ok := g.match(r.URL.Path); ok && route.Upstream != "" {
			u, err := url.Parse(route.Upstream)
			if err != nil {
				http.Error(w, "bad upstream", http.StatusBadGateway)
				return
			}
			proxy := httputil.NewSingleHostReverseProxy(u)
			// Strip prefix before forwarding.
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimPrefix(r.URL.Path, route.Prefix)
			if r2.URL.Path == "" {
				r2.URL.Path = "/"
			}
			proxy.ServeHTTP(w, r2)
			return
		}
		base.ServeHTTP(w, r)
	})
}

func (g *Gateway) match(path string) (Route, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var best Route
	bestLen := -1
	for _, rt := range g.routes {
		if path == rt.Prefix || strings.HasPrefix(path, rt.Prefix+"/") || strings.HasPrefix(path, rt.Prefix) {
			if len(rt.Prefix) > bestLen {
				best = rt
				bestLen = len(rt.Prefix)
			}
		}
	}
	if bestLen < 0 {
		return Route{}, false
	}
	return best, true
}

// Stats augments limiter stats with route count.
func (g *Gateway) Stats() map[string]any {
	if g == nil {
		return map[string]any{"enabled": false}
	}
	st := g.Limiter.Stats()
	st["routes"] = len(g.Routes())
	st["route_table"] = g.Routes()
	return st
}
