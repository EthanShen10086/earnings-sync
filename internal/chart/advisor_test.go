package chart

import (
	"strings"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

func TestRecommendAndInstruction(t *testing.T) {
	ex := sample()
	got := Recommend(ex)
	if len(got) < 4 {
		t.Fatalf("specs %d", len(got))
	}
	pie := FromInstruction("画一个营收构成饼图", ex)
	if len(pie) == 0 || pie[0].ChartType != "PIE" {
		t.Fatalf("pie %#v", pie)
	}
	col := FromInstruction("看总营收柱状图", ex)
	if len(col) != 1 || col[0].Title == "" {
		t.Fatalf("column %#v", col)
	}
}

func TestRecommendFollowsConfiguredSegments(t *testing.T) {
	vals := map[string]float64{"productivity_revenue": 80, "intelligent_cloud_revenue": 40, "total_revenue": 120, "net_income": 30}
	ex := model.Extraction{Ticker: "MSFT", Columns: map[string]string{
		"productivity_revenue": "D", "intelligent_cloud_revenue": "E",
	}}
	for k, v := range vals {
		n := v
		ex.Facts = append(ex.Facts, model.Fact{LineItem: k, Value: &n})
	}
	var titles []string
	for _, spec := range Recommend(ex) {
		titles = append(titles, spec.Title)
	}
	joined := strings.Join(titles, "\n")
	if !strings.Contains(joined, "本季业务构成") || !strings.Contains(joined, "各业务收入怎么变") {
		t.Fatal(joined)
	}
	if strings.Contains(joined, "iPhone") || strings.Contains(joined, "硬件") || strings.Contains(joined, "AWS") {
		t.Fatal(joined)
	}
}
