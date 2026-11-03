package extract

import (
	"os"
	"testing"
)

func TestStructuredFixture(t *testing.T) {
	b, err := os.ReadFile("../../testdata/aapl_2026q4.csv")
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := TryStructured(string(b), "aapl_2026q4.csv")
	if !ok {
		t.Fatal("expected structured parse")
	}
	if ex.Period.Label() != "2026Q4" {
		t.Fatalf("period %s", ex.Period.Label())
	}
	rev, ok := ex.Number("total_revenue")
	if !ok || rev != 114.48 {
		t.Fatalf("revenue %v %v", rev, ok)
	}
	gm, ok := ex.Number("gross_margin")
	if !ok || gm != 0.479 {
		t.Fatalf("margin %v", gm)
	}
}

func TestScaleMillionsAndPercent(t *testing.T) {
	text := "fiscal_year,fiscal_quarter,line_item,value,unit\n2026,4,total_revenue,114480,million_usd\n2026,4,iphone_yoy,17,ratio\n"
	ex, ok := TryStructured(text, "x.csv")
	if !ok {
		t.Fatal("parse")
	}
	rev, _ := ex.Number("total_revenue")
	if rev < 114 || rev > 115 {
		t.Fatalf("scaled revenue %v", rev)
	}
	yoy, _ := ex.Number("iphone_yoy")
	if yoy < 0.16 || yoy > 0.18 {
		t.Fatalf("yoy %v", yoy)
	}
}

func TestStructuredKeepsEveryQuarter(t *testing.T) {
	text := "fiscal_year,fiscal_quarter,line_item,value,unit\n2026,3,total_revenue,90,billion_usd\n2026,4,total_revenue,114.48,billion_usd\n"
	all, ok := TryStructuredAll(text, "x.csv")
	if !ok || len(all) != 2 || all[0].Period.Label() != "2026Q3" || all[1].Period.Label() != "2026Q4" {
		t.Fatalf("%v %+v", ok, all)
	}
	latest, ok := TryStructured(text, "x.csv")
	if !ok || latest.Period.Label() != "2026Q4" {
		t.Fatalf("latest %+v", latest)
	}
}
