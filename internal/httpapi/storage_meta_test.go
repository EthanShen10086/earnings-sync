package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/auth"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
)

func TestMetaExposesStorage(t *testing.T) {
	api := API{
		Obs:  observe.New(),
		Auth: auth.Config{Mode: auth.ModeBypass, DevSubject: "t", DevRoles: []string{"reader"}},
		Storage: StorageInfo{
			WantMode:   "dual",
			Mode:       "sheets-only",
			FailPolicy: "all",
			SheetsKind: "memory",
			PGKind:     "none",
			Notes:      []string{"DATABASE_URL unset"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["api_version"] != APIContractVersion {
		t.Fatalf("api_version %#v", out["api_version"])
	}
	storage, _ := out["storage"].(map[string]any)
	if storage["want_mode"] != "dual" || storage["mode"] != "sheets-only" {
		t.Fatalf("storage %#v", storage)
	}

	stReq := httptest.NewRequest(http.MethodGet, "/v1/platform/status", nil)
	stRec := httptest.NewRecorder()
	api.Handler().ServeHTTP(stRec, stReq)
	if stRec.Code != http.StatusOK {
		t.Fatalf("platform status %d %s", stRec.Code, stRec.Body.String())
	}
	var st map[string]any
	if err := json.Unmarshal(stRec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	stStorage, _ := st["storage"].(map[string]any)
	if stStorage["sheets_kind"] != "memory" {
		t.Fatalf("platform storage %#v", stStorage)
	}
}
