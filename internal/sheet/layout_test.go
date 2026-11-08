package sheet

import (
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

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

func TestMappedLayoutDoesNotUseAppleColumns(t *testing.T) {
	ex := model.Extraction{
		MappedOnly: true,
		Cells: map[string]string{
			"short_term_investments": "资产负债表!G",
			"insurance_underwriting": "利润表!D",
		},
	}
	ref, ok := locate("short_term_investments", ex)
	if !ok || ref.Sheet != "资产负债表" || ref.Col != "G" {
		t.Fatalf("treasury %+v %v", ref, ok)
	}
	if _, ok := locate("cash", ex); ok {
		t.Fatal("unmapped cash must not fall through to Apple's balance-sheet column")
	}
	apple, ok := locate("total_revenue", model.Extraction{})
	if !ok || apple.Sheet != "利润表" || apple.Col != "J" {
		t.Fatalf("apple revenue %+v", apple)
	}
}
