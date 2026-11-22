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
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/adapters/mq"
	"github.com/EthanShen10086/earnings-sync/internal/auth"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/features"
	"github.com/EthanShen10086/earnings-sync/internal/infra"
	"github.com/EthanShen10086/earnings-sync/internal/job"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func TestIngestRunsPlatformSideEffectsWithStubs(t *testing.T) {
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
	dir := t.TempDir()
	stack, err := infra.Open(context.Background(), config.Settings{
		ObjectStoreDir:   filepath.Join(dir, "obj"),
		AuditLogPath:     filepath.Join(dir, "audit.jsonl"),
		SearchIndexPath:  filepath.Join(dir, "search"),
		RateLimitRPS:     100,
		RateLimitBurst:   20,
		KafkaEventLog:    filepath.Join(dir, "kafka.jsonl"),
		LogShipperPath:   filepath.Join(dir, "logs", "app.jsonl"),
		DiscoveryBackend: "memory",
		AlertRulesPath:   filepath.Join("..", "..", "deploy", "alert-rules.yml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stack.Close()

	feat := features.Resolve(features.Flags{
		Redis:         features.WantOn,
		MQ:            features.WantOn,
		Kafka:         features.WantOn,
		ObjectStorage: features.WantOn,
		Audit:         features.WantOn,
		Search:        features.WantOn,
		Discovery:     features.WantOn,
		ELK:           features.WantOn,
	}, features.Deps{}) // no remote URLs → degraded runnable stubs

	delivered := make(chan string, 1)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = stack.Queue.Subscribe(subCtx, "ingest", func(m mq.Message) error {
			delivered <- string(m.Body)
			return nil
		})
	}()
	time.Sleep(20 * time.Millisecond)

	memSheets := &sheet.Memory{}
	api := API{
		Obs:  observe.New(),
		Auth: auth.Config{Mode: auth.ModeBypass, DevSubject: "tester", DevRoles: []string{"admin"}},
		Pipe: &pipeline.Pipeline{
			Companies: reg,
			Jobs:      store,
			Sheets:    memSheets,
			Obs:       observe.New(),
		},
		Facts: memSheets,
		Infra: stack,
		Feat:  feat,
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ticker", "AAPL")
	_ = w.WriteField("dry_run", "false")
	_ = w.WriteField("force", "true")
	part, _ := w.CreateFormFile("file", "aapl_2026q4.csv")
	_, _ = part.Write(body)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status %d %s", rec.Code, rec.Body.String())
	}

	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("mq stub did not deliver ingest event")
	}

	events, err := stack.Audit.List(context.Background(), 10)
	if err != nil || len(events) == 0 {
		t.Fatalf("audit not written: err=%v len=%d", err, len(events))
	}
	hits, err := stack.Search.Query(context.Background(), "AAPL", 5)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search not indexed: err=%v hits=%d", err, len(hits))
	}
	if _, ok, err := stack.Cache.Get(context.Background(), "facts:AAPL"); err != nil || !ok {
		t.Fatalf("cache miss after ingest: ok=%v err=%v", ok, err)
	}
	if stack.Kafka == nil || stack.Kafka.Len() == 0 {
		t.Fatal("kafka stub bus empty")
	}
	if stack.Shipper == nil || stack.Shipper.Status().Sent < 1 {
		t.Fatal("elk shipper did not record ingest log")
	}
	inst, err := stack.Discovery.Discover(context.Background(), "earnings-sync")
	if err != nil || len(inst) == 0 {
		t.Fatalf("discovery empty: %v %+v", err, inst)
	}

	rc, _, err := stack.Objects.Get(context.Background(), "filings/AAPL/aapl_2026q4.csv")
	if err != nil {
		t.Fatalf("object get: %v", err)
	}
	_ = rc.Close()

	searchReq := httptest.NewRequest(http.MethodGet, "/v1/search?q=AAPL", nil)
	searchRec := httptest.NewRecorder()
	api.Handler().ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK || !bytes.Contains(searchRec.Body.Bytes(), []byte(`"hits"`)) {
		t.Fatalf("search endpoint %d %s", searchRec.Code, searchRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
	auditRec := httptest.NewRecorder()
	api.Handler().ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK || !bytes.Contains(auditRec.Body.Bytes(), []byte("ingest")) {
		t.Fatalf("audit endpoint %d %s", auditRec.Code, auditRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/v1/platform/status", nil)
	statusRec := httptest.NewRecorder()
	api.Handler().ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("platform status %d", statusRec.Code)
	}
	var status map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	active, _ := status["active"].(map[string]any)
	if active["audit"] != true || active["search"] != true {
		t.Fatalf("active flags: %#v", active)
	}

	factsReq := httptest.NewRequest(http.MethodGet, "/v1/facts?ticker=AAPL", nil)
	factsRec := httptest.NewRecorder()
	api.Handler().ServeHTTP(factsRec, factsReq)
	if factsRec.Code != http.StatusOK {
		t.Fatalf("facts %d %s", factsRec.Code, factsRec.Body.String())
	}
	if !bytes.Contains(factsRec.Body.Bytes(), []byte(`"cache_hit":true`)) {
		t.Fatalf("expected cache_hit after ingest: %s", factsRec.Body.String())
	}
}

func TestPlatformInactiveWhenFlagsOff(t *testing.T) {
	api := API{
		Obs:  observe.New(),
		Auth: auth.Config{Mode: auth.ModeBypass, DevSubject: "t", DevRoles: []string{"admin"}},
		Feat: features.Resolve(features.Flags{}, features.Deps{}),
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=x", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when search off, got %d", rec.Code)
	}
}
