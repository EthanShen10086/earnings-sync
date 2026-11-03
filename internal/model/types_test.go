package model

import "testing"

func TestPeriodLabel(t *testing.T) {
	p := Period{FiscalYear: 2026, FiscalQuarter: 4}
	if p.Label() != "2026Q4" || !p.Valid() {
		t.Fatalf("period: %+v %s", p, p.Label())
	}
	if (Period{}).Valid() {
		t.Fatal("empty period should be invalid")
	}
}

func TestExtractionLookup(t *testing.T) {
	v := 114.48
	ex := Extraction{Facts: []Fact{{LineItem: "total_revenue", Value: &v}}}
	got, ok := ex.Number("total_revenue")
	if !ok || got != v {
		t.Fatalf("got %v %v", got, ok)
	}
	if _, ok := ex.Number("missing"); ok {
		t.Fatal("missing fact")
	}
}
