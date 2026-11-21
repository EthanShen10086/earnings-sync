package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/auth"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/job"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func TestListFacts(t *testing.T) {
	mem := &sheet.Memory{}
	v := 119.6
	if _, err := mem.Upsert(context.Background(), "wb", model.Extraction{
		Ticker: "AAPL",
		Period: model.Period{FiscalYear: 2026, FiscalQuarter: 4},
		Facts:  []model.Fact{{LineItem: "total_revenue", Value: &v}},
	}); err != nil {
		t.Fatal(err)
	}
	api := API{Obs: observe.New(), Facts: mem, Auth: auth.Config{Mode: auth.ModeBypass, DevSubject: "t", DevRoles: []string{"reader"}}}
	req := httptest.NewRequest(http.MethodGet, "/v1/facts?ticker=AAPL", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Facts []sheet.FactRow `json:"facts"`
		Meta  map[string]any  `json:"meta"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Facts) == 0 {
		t.Fatal("expected facts")
	}
	if out.Meta["api_version"] != APIContractVersion {
		t.Fatalf("meta %#v", out.Meta)
	}
}

func TestMetaAndAuthMe(t *testing.T) {
	api := API{
		Obs:  observe.New(),
		Auth: auth.Config{Mode: auth.ModeBypass, DevSubject: "dev", DevEmail: "d@local", DevRoles: []string{"admin"}, DevTenant: "local"},
	}
	h := api.Handler()
	metaReq := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	metaRec := httptest.NewRecorder()
	h.ServeHTTP(metaRec, metaReq)
	if metaRec.Code != http.StatusOK || !bytes.Contains(metaRec.Body.Bytes(), []byte(APIContractVersion)) {
		t.Fatalf("meta %d %s", metaRec.Code, metaRec.Body.String())
	}
	meReq := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	meRec := httptest.NewRecorder()
	h.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK || !bytes.Contains(meRec.Body.Bytes(), []byte(`"dev"`)) {
		t.Fatalf("me %d %s", meRec.Code, meRec.Body.String())
	}
	alertsReq := httptest.NewRequest(http.MethodGet, "/v1/ops/alerts", nil)
	alertsRec := httptest.NewRecorder()
	h.ServeHTTP(alertsRec, alertsReq)
	if alertsRec.Code != http.StatusOK || !bytes.Contains(alertsRec.Body.Bytes(), []byte(`"alerts"`)) {
		t.Fatalf("alerts %d %s", alertsRec.Code, alertsRec.Body.String())
	}
}

func TestIngestAndGetJob(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "aapl_2026q4.csv"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := config.Load(filepath.Join("..", "..", "configs", "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := job.Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	api := API{
		Obs:  observe.New(),
		Auth: auth.Config{Mode: auth.ModeBypass, DevSubject: "t", DevRoles: []string{"admin"}},
		Pipe: &pipeline.Pipeline{
			Companies: reg,
			Jobs:      store,
			Sheets:    &sheet.Memory{},
			Obs:       observe.New(),
		},
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ticker", "AAPL")
	_ = w.WriteField("dry_run", "true")
	part, _ := w.CreateFormFile("file", "aapl_2026q4.csv")
	_, _ = part.Write(body)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("2026Q4")) {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+payload.Job.ID, nil)
	got := httptest.NewRecorder()
	api.Handler().ServeHTTP(got, get)
	if got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(payload.Job.ID)) {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}
}
