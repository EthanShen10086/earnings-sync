package macro

import (
	"strings"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

func TestParseLongAndWide(t *testing.T) {
	long := "period,basis,indicator,value\n202610,ytd,gdp,4.6\n202610,累计同比,社会消费品零售总额,1.0%\n202610,yoy,pmi,49.3\n202610,mom,社零,0.02\n"
	points, warnings, err := Parse(long)
	if err != nil || len(warnings) != 0 || len(points) != 4 {
		t.Fatalf("long %+v %v %v", points, warnings, err)
	}
	wide := "2022-2026年中国宏观经济数据汇总\n累计同比,202610,202312\nGDP增速,4.6%,5.2%\n扣除房地产开发投资,-5.1%,7.2%\n民间投资,-11.4%,-0.4%\n扣除房地产开发投资,-7.8%,6.0%\n当期同比,202610\nPMI,49.3\n"
	points, warnings, err = Parse(wide)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("wide warnings %v err %v", warnings, err)
	}
	if !has(points, BasisYTD, "fai_ex_property", "202312", 7.2) || !has(points, BasisYTD, "private_ex_property", "202610", -7.8) {
		t.Fatalf("duplicate rows %+v", points)
	}
	if !has(points, BasisYoY, "pmi", "202610", 49.3) {
		t.Fatalf("pmi %+v", points)
	}
}

func TestRenderKeepsOlderMonth(t *testing.T) {
	first := Render("2022-2026年中国宏观经济数据汇总", nil, []model.MacroPoint{
		{Period: "202610", Basis: BasisYTD, Indicator: "gdp", Value: ptr(4.6)},
	})
	second := Render("", first.Values, []model.MacroPoint{
		{Period: "202312", Basis: BasisYTD, Indicator: "gdp", Value: ptr(5.2)},
	})
	header := second.Values[1]
	if header[1] != "202312" || header[2] != "202610" {
		t.Fatalf("header %#v", header)
	}
	gdp := second.Values[2]
	if gdp[0] != "GDP增速" || gdp[1] != 5.2 || gdp[2] != 4.6 {
		t.Fatalf("gdp %#v", gdp)
	}
	if len(second.Cells) != 1 || second.Cells[0].Value != 5.2 {
		t.Fatalf("cells %+v", second.Cells)
	}
	if second.Values[0][0] != "2022-2026年中国宏观经济数据汇总" {
		t.Fatal(second.Values[0])
	}
}

func TestSummaryAndCheck(t *testing.T) {
	points := []model.MacroPoint{
		{Period: "202610", Basis: BasisYTD, Indicator: "gdp", Value: ptr(4.6)},
		{Period: "202610", Basis: BasisYTD, Indicator: "retail", Value: ptr(1)},
		{Period: "202610", Basis: BasisYTD, Indicator: "cpi", Value: ptr(0.9)},
		{Period: "202610", Basis: BasisYTD, Indicator: "ppi", Value: ptr(2.7)},
		{Period: "202610", Basis: BasisYTD, Indicator: "cpi_ppi", Value: ptr(-1.8)},
		{Period: "202610", Basis: BasisYoY, Indicator: "pmi", Value: ptr(49.3)},
	}
	lines := Summary(points)
	if len(lines) != 2 || !strings.Contains(lines[0], "GDP增速 4.6%") || !strings.Contains(lines[1], "PMI 49.3") {
		t.Fatal(lines)
	}
	warnings := Check(points)
	if len(warnings) != 0 {
		t.Fatal(warnings)
	}
	points[4].Value = ptr(5)
	if len(Check(points)) != 1 {
		t.Fatal(Check(points))
	}
}

func has(points []model.MacroPoint, basis, key, period string, want float64) bool {
	for _, pt := range points {
		if pt.Basis == basis && pt.Indicator == key && pt.Period == period && pt.Value != nil && *pt.Value == want {
			return true
		}
	}
	return false
}

func ptr(n float64) *float64 { return &n }
