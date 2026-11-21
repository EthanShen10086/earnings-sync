package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBypassInjectsPrincipal(t *testing.T) {
	cfg := Config{Mode: ModeBypass, DevSubject: "u1", DevRoles: []string{"reader"}, DevTenant: "t"}
	var got Principal
	h := Middleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("missing principal")
		}
		got = p
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/facts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if got.Subject != "u1" || got.Source != ModeBypass {
		t.Fatalf("got %+v", got)
	}
}

func TestBearerRejectsMissingToken(t *testing.T) {
	cfg := Config{Mode: ModeBearer, BearerToken: "secret"}
	h := Middleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/facts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestBearerAcceptsToken(t *testing.T) {
	cfg := Config{Mode: ModeBearer, BearerToken: "secret", DevSubject: "tok-user", DevRoles: []string{"admin"}}
	h := Middleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := FromContext(r.Context())
		if p.Subject != "tok-user" || p.Source != ModeBearer {
			t.Fatalf("principal %+v", p)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/facts", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestPublicPathsSkipAuth(t *testing.T) {
	cfg := Config{Mode: ModeBearer, BearerToken: "secret"}
	called := false
	h := Middleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("public path blocked: called=%v status=%d", called, rec.Code)
	}
}

func TestOIDCStubRejects(t *testing.T) {
	cfg := Config{Mode: ModeOIDC}
	_, err := Authenticate(cfg, httptest.NewRequest(http.MethodGet, "/v1/facts", nil))
	if err == nil {
		t.Fatal("expected error")
	}
}
