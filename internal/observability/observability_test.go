package observability_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/observability"
)

func TestShipperFileAndESStub(t *testing.T) {
	dir := t.TempDir()
	es := httptest.NewServer(observability.ESStubHandler())
	defer es.Close()

	path := filepath.Join(dir, "app.jsonl")
	ship, st, err := observability.OpenShipper(path, es.URL, "earnings-sync", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer ship.Close()
	if st.Backend != "file+es" {
		t.Fatalf("status=%+v", st)
	}
	ctx := context.Background()
	if err := ship.Ship(ctx, observability.LogEvent{
		Level:   "info",
		Message: "hello elk",
		Fields:  map[string]string{"job": "1"},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		t.Fatalf("file empty: %v", err)
	}
	var line map[string]any
	if err := json.Unmarshal(bytesTrimLine(raw), &line); err != nil {
		t.Fatal(err)
	}
	if line["message"] != "hello elk" {
		t.Fatalf("line=%v", line)
	}
	resp, err := http.Get(es.URL + "/earnings-sync/_count")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var count struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&count); err != nil {
		t.Fatal(err)
	}
	if count.Count != 1 {
		t.Fatalf("es count=%d", count.Count)
	}
	if ship.Status().Sent < 1 {
		t.Fatal("sent counter")
	}
}

func TestShipperFileOnlyWhenNoES(t *testing.T) {
	path := filepath.Join(t.TempDir(), "only.jsonl")
	ship, st, err := observability.OpenShipper(path, "", "idx", "svc")
	if err != nil {
		t.Fatal(err)
	}
	defer ship.Close()
	if st.Degraded || st.Backend != "file" {
		t.Fatalf("%+v", st)
	}
	_ = ship.Ship(context.Background(), observability.LogEvent{Message: "x", Level: "info", Timestamp: time.Now().UTC()})
}

func TestLoadAlertRules(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "alert-rules.yml")
	rules, err := observability.LoadAlertRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 2 {
		t.Fatalf("expected rules from deploy/alert-rules.yml, got %d", len(rules))
	}
	if rules[0].Alert == "" {
		t.Fatal("empty alert name")
	}
}

func bytesTrimLine(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
