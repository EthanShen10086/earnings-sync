package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/job"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

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
		Obs: observe.New(),
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
