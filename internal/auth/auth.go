// Package auth provides IAM/SSO placeholders for the HTTP API.
// Default AUTH_MODE=bypass keeps demos runnable without real IdP secrets.
package auth

import (
	"context"
	"net/http"
	"os"
	"strings"
)

type contextKey struct{}

// Mode selects how requests are authenticated.
const (
	ModeBypass = "bypass" // local/dev: inject a synthetic principal
	ModeBearer = "bearer" // shared static token (demo secret)
	ModeOIDC   = "oidc"   // placeholder shape only; rejects until wired
)

// Principal is the caller identity attached to the request context.
type Principal struct {
	Subject string   `json:"sub"`
	Email   string   `json:"email,omitempty"`
	Name    string   `json:"name,omitempty"`
	Roles   []string `json:"roles"`
	Tenant  string   `json:"tenant,omitempty"`
	Source  string   `json:"source"` // bypass | bearer | oidc
}

// Config is loaded from environment (see LoadConfig).
type Config struct {
	Mode        string
	DevSubject  string
	DevEmail    string
	DevName     string
	DevRoles    []string
	DevTenant   string
	BearerToken string
	// OIDC placeholders — shape only; unused in bypass/bearer.
	OIDCIssuer   string
	OIDCClientID string
	OIDCAudience string
}

// LoadConfig reads AUTH_* env vars. Missing values default to safe local demos.
func LoadConfig() Config {
	mode := strings.ToLower(strings.TrimSpace(env("AUTH_MODE", ModeBypass)))
	switch mode {
	case ModeBypass, ModeBearer, ModeOIDC:
	default:
		mode = ModeBypass
	}
	roles := splitCSV(env("AUTH_DEV_ROLES", "admin,reader"))
	return Config{
		Mode:         mode,
		DevSubject:   env("AUTH_DEV_SUBJECT", "dev-user"),
		DevEmail:     env("AUTH_DEV_EMAIL", "dev@localhost"),
		DevName:      env("AUTH_DEV_NAME", "Dev Bypass"),
		DevRoles:     roles,
		DevTenant:    env("AUTH_DEV_TENANT", "local"),
		BearerToken:  os.Getenv("AUTH_BEARER_TOKEN"),
		OIDCIssuer:   os.Getenv("AUTH_OIDC_ISSUER"),
		OIDCClientID: os.Getenv("AUTH_OIDC_CLIENT_ID"),
		OIDCAudience: os.Getenv("AUTH_OIDC_AUDIENCE"),
	}
}

// Providers lists SSO/IdP placeholders for discovery UIs.
func (c Config) Providers() []map[string]any {
	oidcReady := c.OIDCIssuer != "" && c.OIDCClientID != ""
	return []map[string]any{
		{
			"id":          "dev-bypass",
			"type":        "bypass",
			"enabled":     c.Mode == ModeBypass,
			"description": "Local synthetic principal; no secrets required",
		},
		{
			"id":          "static-bearer",
			"type":        "bearer",
			"enabled":     c.Mode == ModeBearer,
			"description": "Authorization: Bearer <AUTH_BEARER_TOKEN>",
		},
		{
			"id":          "oidc",
			"type":        "oidc",
			"enabled":     c.Mode == ModeOIDC,
			"ready":       oidcReady,
			"issuer":      c.OIDCIssuer,
			"client_id":   c.OIDCClientID,
			"audience":    c.OIDCAudience,
			"description": "OIDC/SSO placeholder — wire real IdP before production",
		},
	}
}

// Middleware authenticates requests and stores Principal in context.
// Public paths (health, metrics, OPTIONS, auth discovery) skip auth.
func Middleware(cfg Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skipAuth(r) {
			next.ServeHTTP(w, r)
			return
		}
		p, err := Authenticate(cfg, r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", challenge(cfg))
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"` + err.Error() + `"}`))
			return
		}
		ctx := context.WithValue(r.Context(), contextKey{}, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Authenticate resolves a Principal for the request.
func Authenticate(cfg Config, r *http.Request) (Principal, error) {
	switch cfg.Mode {
	case ModeBearer:
		tok := bearerToken(r)
		if cfg.BearerToken == "" {
			return Principal{}, errAuth("AUTH_BEARER_TOKEN not configured")
		}
		if tok == "" || tok != cfg.BearerToken {
			return Principal{}, errAuth("invalid or missing bearer token")
		}
		return Principal{
			Subject: cfg.DevSubject,
			Email:   cfg.DevEmail,
			Name:    cfg.DevName,
			Roles:   append([]string(nil), cfg.DevRoles...),
			Tenant:  cfg.DevTenant,
			Source:  ModeBearer,
		}, nil
	case ModeOIDC:
		// Shape reserved for future OIDC code exchange / JWT validation.
		if bearerToken(r) == "" {
			return Principal{}, errAuth("oidc mode requires Authorization Bearer (not wired; use AUTH_MODE=bypass for demos)")
		}
		return Principal{}, errAuth("oidc validation not implemented; set AUTH_MODE=bypass or bearer")
	default: // bypass
		return Principal{
			Subject: cfg.DevSubject,
			Email:   cfg.DevEmail,
			Name:    cfg.DevName,
			Roles:   append([]string(nil), cfg.DevRoles...),
			Tenant:  cfg.DevTenant,
			Source:  ModeBypass,
		}, nil
	}
}

// FromContext returns the authenticated principal, if any.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

func skipAuth(r *http.Request) bool {
	if r.Method == http.MethodOptions {
		return true
	}
	path := r.URL.Path
	switch path {
	case "/healthz", "/metrics", "/v1/meta", "/v1/auth/providers":
		return true
	}
	return false
}

func challenge(cfg Config) string {
	switch cfg.Mode {
	case ModeBearer, ModeOIDC:
		return `Bearer realm="earnings-sync"`
	default:
		return `Bearer realm="earnings-sync", bypass`
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const p = "Bearer "
	if len(h) < len(p) || !strings.EqualFold(h[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

type authError string

func (e authError) Error() string { return string(e) }

func errAuth(msg string) error { return authError(msg) }

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"reader"}
	}
	return out
}
