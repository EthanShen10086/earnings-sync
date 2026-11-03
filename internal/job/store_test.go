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
	j := s.Create("AAPL", "abc", "https://example.test/a", "url", true)
	j.Status = model.StatusSucceeded
	if err := s.Save(j); err != nil {
		t.Fatal(err)
	}
	got, ok := s.ByHash("abc")
	if !ok || got.ID != j.ID {
		t.Fatalf("hash lookup %+v", got)
	}
	got, ok = s.ByLocator("https://example.test/a")
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
