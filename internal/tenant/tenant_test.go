package tenant_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/tenant"
)

func TestRegistryAndMiddleware(t *testing.T) {
	reg := tenant.NewRegistry()
	if err := reg.Upsert(tenant.Tenant{ID: "acme", Name: "Acme", Plan: "pro", Active: true, QuotaRPS: 100}); err != nil {
		t.Fatal(err)
	}

	var saw string
	h := reg.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tn, ok := tenant.FromContext(r.Context())
		if !ok {
			t.Fatal("missing tenant")
		}
		saw = tn.ID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/facts?ticker=AAPL", nil)
	req.Header.Set("X-Tenant-ID", "acme")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 || saw != "acme" {
		t.Fatalf("code=%d saw=%s", rr.Code, saw)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/facts", nil)
	req2.Header.Set("X-Tenant-ID", "missing")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 404 {
		t.Fatalf("want 404 got %d", rr2.Code)
	}
}
