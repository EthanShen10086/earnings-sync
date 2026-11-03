package chart

import (
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Recommend returns rule-based charts for the facts that are present.
func Recommend(ex model.Extraction) []model.ChartSpec {
	var out []model.ChartSpec
	add := func(item, title, reason string) {
		if _, ok := ex.Number(item); ok {
			out = append(out, model.ChartSpec{
				Title: title, ChartType: model.ChartColumn, Reason: reason, Source: model.SourceRule, Sheet: sheetOf(item),
			})
		}
	}
	add("total_revenue", "总营收（已披露期间）", "单一金额按财季比较，柱状图能直接看出高低")
	add("net_income", "净利润（已披露期间）", "利润和营收分开看，避免被收入规模盖住")
	add("gross_margin", "毛利率（已披露期间）", "比率用同一根轴看趋势，不和金额混在一张图")
	add("iphone_revenue", "iPhone营收（已披露期间）", "iPhone 是收入主体，单独成图")
	add("services_revenue", "服务营收（已披露期间）", "服务是另一条增长曲线")
	add("iphone_units", "iPhone出货量（已披露期间，百万台）", "出货量单位不同，不能和营收画在一起")
	add("capex", "资本开支（已披露期间）", "资本开支按已披露季度排列")
	add("free_cash_flow", "自由现金流（已披露期间）", "自由现金流和资本开支分开，避免互相遮挡")
	if segmentCount(ex) >= 2 {
		out = append(out, model.ChartSpec{
			Title: "所选季度营收构成", ChartType: model.ChartPie, Reason: "同一季度的业务构成用饼图看占比",
			Source: model.SourceRule, Sheet: "利润表",
		})
	}
	if hasAny(ex, "iphone_yoy", "mac_yoy", "ipad_yoy", "wearables_yoy", "services_yoy") {
		out = append(out, model.ChartSpec{
			Title: "所选季度分业务同比", ChartType: model.ChartColumn, Reason: "同比有正有负，柱状图能同时看到增长和下降",
			Source: model.SourceRule, Sheet: "利润表",
		})
	}
	return out
}

// FromInstruction maps a short Chinese or English instruction onto known charts.
func FromInstruction(text string, ex model.Extraction) []model.ChartSpec {
	text = strings.ToLower(text)
	all := Recommend(ex)
	if strings.Contains(text, "饼") || strings.Contains(text, "构成") || strings.Contains(text, "mix") {
		return filterType(all, model.ChartPie)
	}
	if strings.Contains(text, "同比") || strings.Contains(text, "yoy") {
		return filterTitle(all, "同比")
	}
	switch {
	case strings.Contains(text, "资本开支") || strings.Contains(text, "capex"):
		return filterTitle(all, "资本开支")
	case strings.Contains(text, "现金流") || strings.Contains(text, "fcf"):
		return filterTitle(all, "自由现金流")
	case strings.Contains(text, "出货"):
		return filterTitle(all, "出货")
	case strings.Contains(text, "毛利"):
		return filterTitle(all, "毛利率")
	case strings.Contains(text, "净利润") || strings.Contains(text, "profit"):
		return filterTitle(all, "净利润")
	case strings.Contains(text, "iphone"):
		return filterTitle(all, "iPhone营收")
	case strings.Contains(text, "服务"):
		return filterTitle(all, "服务")
	case strings.Contains(text, "营收") || strings.Contains(text, "revenue") || strings.Contains(text, "柱"):
		return filterTitle(all, "总营收")
	default:
		return nil
	}
}

func segmentCount(ex model.Extraction) int {
	n := 0
	for _, item := range []string{"iphone_revenue", "mac_revenue", "ipad_revenue", "wearables_revenue", "services_revenue"} {
		if _, ok := ex.Number(item); ok {
			n++
		}
	}
	return n
}

func hasAny(ex model.Extraction, items ...string) bool {
	for _, item := range items {
		if _, ok := ex.Number(item); ok {
			return true
		}
	}
	return false
}

func filterType(in []model.ChartSpec, chartType string) []model.ChartSpec {
	var out []model.ChartSpec
	for _, c := range in {
		if c.ChartType == chartType {
			out = append(out, c)
		}
	}
	return out
}

func filterTitle(in []model.ChartSpec, part string) []model.ChartSpec {
	var out []model.ChartSpec
	for _, c := range in {
		if strings.Contains(c.Title, part) {
			out = append(out, c)
		}
	}
	return out
}

func sheetOf(item string) string {
	switch item {
	case "capex", "free_cash_flow":
		return "现金流量表"
	default:
		return "利润表"
	}
}
