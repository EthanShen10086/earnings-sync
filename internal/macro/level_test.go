package macro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/model"
)

func TestJanOctLevelsMatchTheTable(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cn_jan_oct.csv"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := config.LoadMacro(filepath.Join("..", "..", "configs", "macro.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	series, ok := reg.Get("CN")
	if !ok {
		t.Fatal("missing CN")
	}
	rel, warnings, err := Read(string(body), series.Panels)
	if err != nil || len(warnings) != 0 || len(rel.Levels) != 27*5 {
		t.Fatalf("levels %d warnings %v err %v", len(rel.Levels), warnings, err)
	}
	rel.Panels = series.Panels
	lines := Brief(rel)
	if !hasLine(lines, "2026年1-10月 固定资产投资完成额 36.48万亿元，同比 -10.79%") {
		t.Fatal(lines)
	}
	if !hasLine(lines, "社会消费品零售总额 41.65万亿元，同比 1.04%，较历年同期最高 0.00%") {
		t.Fatal(lines)
	}
	panel := series.Panels[1]
	sheet := RenderLevel(panel, nil, rel.Levels)
	if sheet.Values[28][0] != "同比变动 (%)" {
		t.Fatalf("row %+v", sheet.Values[28])
	}
	formula, _ := sheet.Values[28][1].(string)
	if !strings.Contains(formula, "ROUND((B28-B27)/B27*100,2)") || !strings.Contains(formula, "MAX(B$2:B28)") && !strings.Contains(sheet.Values[30][1].(string), "MAX(B$2:B28)") {
		t.Fatalf("formula %s peak %v", formula, sheet.Values[30][1])
	}
	charts := ChartSpecs(rel)
	if len(charts) != 2 || !strings.Contains(charts[0].Title, "新建商品房销售收入较历年同期最高点减少近9万亿") {
		t.Fatalf("charts %+v", charts)
	}
	if charts[1].Title != "历年1-10月" {
		t.Fatal(charts[1].Title)
	}
	again := RenderLevel(panel, sheet.Values, []model.LevelPoint{{
		Window: "1-10", Year: 2026, Indicator: "fai", Value: ptr(36.48),
	}})
	if again.Values[1][1] != 1.57 || again.Values[27][1] != 36.48 {
		t.Fatalf("kept history %#v %#v", again.Values[1], again.Values[27])
	}
}

func TestUnknownWindowNamesTheMissingPanel(t *testing.T) {
	text := "历年1-9月,固定资产投资完成额（万亿元）\n2000年,1.57\n2001年,1.84\n"
	reg, err := config.LoadMacro(filepath.Join("..", "..", "configs", "macro.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	series, _ := reg.Get("CN")
	_, _, err = Read(text, series.Panels)
	if err == nil || !strings.Contains(err.Error(), "window: 1-9") {
		t.Fatal(err)
	}
}

func hasLine(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}
