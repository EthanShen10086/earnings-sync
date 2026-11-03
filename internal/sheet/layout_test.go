package sheet

import "testing"

func TestInputColumnsSkipFormulas(t *testing.T) {
	ref, ok := inputColumns["total_revenue"]
	if !ok || ref.Sheet != "利润表" || ref.Col != "J" {
		t.Fatalf("%+v", ref)
	}
	if _, ok := inputColumns["net_income"]; !ok {
		t.Fatal("net income should be writable")
	}
	for _, formula := range []string{"segment_total", "gross_profit", "revenue_yoy"} {
		if _, ok := inputColumns[formula]; ok {
			t.Fatalf("formula column %s should not be written", formula)
		}
	}
	if _, ok := templateByTitle("总营收（已披露期间）"); !ok {
		t.Fatal("missing revenue chart template")
	}
}
