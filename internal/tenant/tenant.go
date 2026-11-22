// Package tenant provides multi-tenant identity, context propagation, and a
// registry. Sheets+PG dual-write remains tenant-agnostic at the store layer;
// callers attach TenantID so ORM / lakehouse / canary can scope work.
package tenant

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ctxKey struct{}

// Tenant is one logical customer / workspace.
type Tenant struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Plan      string            `json:"plan"` // free | pro | enterprise
	QuotaRPS  int               `json:"quota_rps"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	Active    bool              `json:"active"`
}

// With stores tenant on context.
func With(ctx context.Context, t Tenant) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext returns tenant if present.
func FromContext(ctx context.Context) (Tenant, bool) {
	t, ok := ctx.Value(ctxKey{}).(Tenant)
	return t, ok
}

// IDFromContext returns tenant id or empty.
func IDFromContext(ctx context.Context) string {
	if t, ok := FromContext(ctx); ok {
		return t.ID
	}
	return ""
}

// Registry is an in-process tenant directory (stub for IAM/SSO wiring).
type Registry struct {
	mu   sync.RWMutex
	byID map[string]Tenant
}

// NewRegistry creates an empty registry with a default "default" tenant.
func NewRegistry() *Registry {
	r := &Registry{byID: map[string]Tenant{}}
	_ = r.Upsert(Tenant{
		ID:        "default",
		Name:      "Default",
		Plan:      "free",
		QuotaRPS:  10,
		Active:    true,
		CreatedAt: time.Now().UTC(),
	})
	return r
}

// Upsert creates or replaces a tenant.
func (r *Registry) Upsert(t Tenant) error {
	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" {
		return fmt.Errorf("tenant: empty id")
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	if t.Plan == "" {
		t.Plan = "free"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[t.ID] = t
	return nil
}

// Get returns a tenant by id.
func (r *Registry) Get(id string) (Tenant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[strings.TrimSpace(id)]
	return t, ok
}

// List returns all tenants.
func (r *Registry) List() []Tenant {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tenant, 0, len(r.byID))
	for _, t := range r.byID {
		out = append(out, t)
	}
	return out
}

// Resolve picks tenant from header X-Tenant-ID or query tenant_id; falls back to default.
func (r *Registry) Resolve(req *http.Request) (Tenant, error) {
	id := strings.TrimSpace(req.Header.Get("X-Tenant-ID"))
	if id == "" {
		id = strings.TrimSpace(req.URL.Query().Get("tenant_id"))
	}
	if id == "" {
		id = "default"
	}
	t, ok := r.Get(id)
	if !ok {
		return Tenant{}, fmt.Errorf("tenant %q not found", id)
	}
	if !t.Active {
		return Tenant{}, fmt.Errorf("tenant %q inactive", id)
	}
	return t, nil
}

// Middleware injects tenant into request context. Unknown tenant → 404 JSON.
func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Skip public probes.
		if req.URL.Path == "/healthz" || req.URL.Path == "/metrics" || req.URL.Path == "/v1/meta" {
			next.ServeHTTP(w, req)
			return
		}
		t, err := r.Resolve(req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"` + err.Error() + `"}`))
			return
		}
		next.ServeHTTP(w, req.WithContext(With(req.Context(), t)))
	})
}
