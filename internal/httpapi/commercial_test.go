package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/auth"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
)

func TestCommercialStateMachinesHTTP(t *testing.T) {
	api := API{
		Obs:  observe.New(),
		Auth: auth.Config{Mode: auth.ModeBypass, DevSubject: "ops", DevRoles: []string{"admin"}},
	}
	h := api.Handler()

	// status
	stReq := httptest.NewRequest(http.MethodGet, "/v1/commercial/status", nil)
	stRec := httptest.NewRecorder()
	h.ServeHTTP(stRec, stReq)
	if stRec.Code != http.StatusOK || !bytes.Contains(stRec.Body.Bytes(), []byte(`"orm_backend"`)) {
		t.Fatalf("status %d %s", stRec.Code, stRec.Body.String())
	}

	// canary lifecycle
	body := []byte(`{"id":"rel-http","flow_id":"earnings-ingest","stable_version":"v1","canary_version":"v2"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/canary/releases", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("canary start %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/canary/releases/rel-http/canary", bytes.NewReader([]byte(`{"percent":100}`)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("canary set %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/canary/releases/rel-http/pick", bytes.NewReader([]byte(`{"key":"u1"}`)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var pick map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&pick); err != nil || pick["bucket"] != "canary" {
		t.Fatalf("pick %#v err=%v", pick, err)
	}

	// HITL
	req = httptest.NewRequest(http.MethodPost, "/v1/hitl/ask", bytes.NewReader([]byte(`{"id":"iv-http","run_id":"run-1","node_id":"n1","prompt":"ok?"}`)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hitl ask %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/hitl/iv-http/decide", bytes.NewReader([]byte(`{"approve":true,"reason":"lgtm"}`)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`approved`)) {
		t.Fatalf("hitl decide %d %s", rec.Code, rec.Body.String())
	}

	// lakehouse
	req = httptest.NewRequest(http.MethodPost, "/v1/lakehouse/facts", bytes.NewReader([]byte(
		`{"facts":[{"ticker":"AAPL","period":"2026Q4","line_item":"revenue","value":1,"event_time":"2026-11-15T00:00:00Z"}]}`,
	)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lake facts %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/lakehouse/exports", bytes.NewReader([]byte(
		`{"id":"exp-http","partition":{"tenant_id":"default","year":2026,"month":11},"sink":"stub://x"}`,
	)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export start %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/lakehouse/exports/exp-http/run", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`succeeded`)) {
		t.Fatalf("export run %d %s", rec.Code, rec.Body.String())
	}

	// SLA drill
	req = httptest.NewRequest(http.MethodPost, "/v1/sla/drills", bytes.NewReader([]byte(`{"id":"drill-http","kind":"restore"}`)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sla schedule %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/sla/drills/drill-http/run", bytes.NewReader([]byte(`{"observed_ms":1000}`)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`passed`)) {
		t.Fatalf("sla run %d %s", rec.Code, rec.Body.String())
	}
}
