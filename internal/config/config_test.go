package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCompanies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "companies.yaml")
	body := []byte(`
companies:
  - ticker: aapl
    name: Apple
    fiscal_year_end_month: 9
    spreadsheet_id: abc
    sources:
      - type: sec_edgar
        cik: "0000320193"
  - ticker: SPACEX
    name: SpaceX
    fiscal_year_end_month: 12
    sources:
      - type: manual
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := reg.Get("aapl")
	if !ok || c.Ticker != "AAPL" || c.SheetURL() == "" {
		t.Fatalf("company %+v", c)
	}
	if len(reg.List()) != 2 {
		t.Fatalf("list %d", len(reg.List()))
	}
}

func TestRejectBadMonth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "companies.yaml")
	if err := os.WriteFile(path, []byte("companies:\n  - ticker: X\n    fiscal_year_end_month: 13\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}
