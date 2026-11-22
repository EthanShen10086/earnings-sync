package gateway_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/adapters/gateway"
)

func TestGatewayRoutesAndRateLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "up:"+r.URL.Path)
	}))
	defer upstream.Close()

	gw := gateway.New(100, 10)
	gw.RegisterRoute(gateway.Route{Prefix: "/proxy", Upstream: upstream.URL, Service: "echo"})
	local := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "local")
	})
	h := gw.Handler(local)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/proxy/x", nil))
	if rr.Code != 200 || rr.Body.String() != "up:/x" {
		t.Fatalf("proxy: code=%d body=%q", rr.Code, rr.Body.String())
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/v1/meta", nil))
	if rr2.Body.String() != "local" {
		t.Fatalf("local=%q", rr2.Body.String())
	}

	st := gw.Stats()
	if st["routes"].(int) != 1 {
		t.Fatalf("stats=%v", st)
	}
}

func TestLimiterStillWorksViaGateway(t *testing.T) {
	gw := gateway.New(1, 1)
	h := gw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:9"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("first=%d", rr.Code)
	}
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second=%d", rr2.Code)
	}
}
