package sheet

import (
	"strings"
	"testing"
)

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

func stringify(v any) string {
	s, _ := v.(string)
	return s
}
