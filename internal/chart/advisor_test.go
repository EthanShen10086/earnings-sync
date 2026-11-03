package chart

import "testing"

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
