package sheet

import (
	"strings"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/model"
)

func TestSegmentColumnOverride(t *testing.T) {
	m := &Memory{}
	n := 45.98
	_, err := m.Upsert(nil, "x", model.Extraction{
		Facts:   []model.Fact{{LineItem: "aws_revenue", Value: &n}},
		Columns: map[string]string{"aws_revenue": "F"},
	})
	if err != nil || len(m.Writes) != 1 || m.Writes[0].A1 != "F5" || m.Writes[0].Sheet != "利润表" {
		t.Fatalf("%+v %v", m.Writes, err)
	}
}

func TestAnalysisFormulasCoverThreeStatements(t *testing.T) {
	var sheetsSeen []string
	for _, rng := range analysisFormulas() {
		sheetsSeen = append(sheetsSeen, rng.Range)
		flat := ""
		for _, row := range rng.Values {
			for _, cell := range row {
				flat += " " + stringify(cell)
			}
		}
		if strings.Contains(rng.Range, "利润表!A120") && !strings.Contains(flat, "硬件") {
			t.Fatal(flat)
		}
		if strings.Contains(rng.Range, "资产负债表!A140") && !strings.Contains(flat, "净现金") {
			t.Fatal(flat)
		}
		if strings.Contains(rng.Range, "现金流量表!A120") && !strings.Contains(flat, "经营现金流") {
			t.Fatal(flat)
		}
	}
	if len(sheetsSeen) < 8 {
		t.Fatalf("ranges %v", sheetsSeen)
	}
	for _, title := range []string{"硬件还是服务：本季收入从哪来", "家底和债务谁更大", "这一季现金怎么来、怎么走"} {
		if _, ok := templateByTitle(title); !ok {
			t.Fatal(title)
		}
	}
}

func TestAnalysisFormulasFollowSegments(t *testing.T) {
	segs := []config.Segment{
		{Key: "productivity_revenue", Column: "D", Header: "生产力与业务流程_十亿美元"},
		{Key: "intelligent_cloud_revenue", Column: "E", Header: "智能云_十亿美元"},
	}
	flat := ""
	for _, rng := range analysisFormulasFor(segs) {
		for _, row := range rng.Values {
			for _, cell := range row {
				flat += " " + stringify(cell)
			}
		}
	}
	if strings.Contains(flat, "硬件") || !strings.Contains(flat, "智能云") || !strings.Contains(flat, "FILTER(E2:E80") {
		t.Fatal(flat)
	}
	trend := segmentTrend(segs)
	if trend.Title != "各业务收入怎么变" || len(trend.SeriesCols) != 2 {
		t.Fatalf("%+v", trend)
	}
}

func stringify(v any) string {
	s, _ := v.(string)
	return s
}
