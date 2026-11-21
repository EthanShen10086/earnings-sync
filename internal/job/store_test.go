package job

import (
	"path/filepath"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

func TestIdempotentHash(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var store Store = s
	j := store.Create("AAPL", "abc", "https://example.test/a", "url", true)
	j.Status = model.StatusSucceeded
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	got, ok := store.ByHash("abc")
	if !ok || got.ID != j.ID {
		t.Fatalf("hash lookup %+v", got)
	}
	got, ok = store.ByLocator("https://example.test/a")
	if !ok || got.Ticker != "AAPL" {
		t.Fatalf("locator lookup %+v", got)
	}
	reopen, err := Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopen.Get(j.ID); !ok {
		t.Fatal("job missing after reopen")
	}
}

func TestOpenAutoFallsBackToFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "jobs.json")
	store, cleanup, err := OpenAuto(t.Context(), OpenOptions{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, ok := store.(*FileStore); !ok {
		t.Fatalf("expected FileStore, got %T", store)
	}
	j := store.Create("MSFT", "h1", "", "file", true)
	if j.ID == "" {
		t.Fatal("empty id")
	}
}
