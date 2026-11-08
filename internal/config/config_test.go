package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/model"
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
	if err := reg.SetSpreadsheet("SPACEX", "sheet-1"); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	sx, ok := again.Get("SPACEX")
	if !ok || sx.SpreadsheetID != "sheet-1" {
		t.Fatalf("saved %+v", sx)
	}
	apple, _ := again.Get("AAPL")
	if apple.SpreadsheetID != "abc" {
		t.Fatalf("apple id changed to %s", apple.SpreadsheetID)
	}
}

func TestAllowedLineItemsStayWithTheCompany(t *testing.T) {
	apple := Company{Ticker: "AAPL"}
	if !strings.Contains(apple.AllowedLineItems(), "iphone_revenue") {
		t.Fatal(apple.AllowedLineItems())
	}
	other := Company{Ticker: "MSFT"}
	if strings.Contains(other.AllowedLineItems(), "iphone") {
		t.Fatal(other.AllowedLineItems())
	}
	amazon := Company{Ticker: "AMZN", Segments: []Segment{{Key: "aws_revenue", Column: "F", Header: "AWS净销售额_十亿美元"}}}
	got := amazon.AllowedLineItems()
	if !strings.Contains(got, "aws_revenue") || strings.Contains(got, "iphone") {
		t.Fatal(got)
	}
}

func TestBerkshireUsesItsOwnCells(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	reg, err := Load(filepath.Join(filepath.Dir(file), "..", "..", "configs", "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	brk, ok := reg.Get("BRK")
	if !ok || !brk.UsesMappedLayout() || brk.SegmentTotal != "disclosed_operating_earnings" {
		t.Fatalf("%+v", brk)
	}
	items := brk.AllowedLineItems()
	if strings.Contains(items, "iphone") || !strings.Contains(items, "insurance_underwriting") || !strings.Contains(items, "ytd_operating_cash_flow") {
		t.Fatal(items)
	}
	cells := brk.InputCells()
	if cells["short_term_investments"] != "资产负债表!G" || cells["bnsf_earnings"] != "利润表!F" || cells["capex"] != "现金流量表!F" {
		t.Fatalf("%v", cells)
	}
	var ex model.Extraction
	brk.Bind(&ex)
	if !ex.MappedOnly || ex.SegmentTotal != "disclosed_operating_earnings" || ex.Columns["bnsf_earnings"] != "F" {
		t.Fatalf("%+v", ex)
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
