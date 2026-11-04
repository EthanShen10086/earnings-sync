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
	if configuredSegments(ex) >= 2 {
		out = append(out, model.ChartSpec{
			Title: "本季业务构成", ChartType: model.ChartPie, Reason: "这一季各业务分部放在一张饼图里。分部不是公司总营收",
			Source: model.SourceRule, Sheet: "利润表",
		}, model.ChartSpec{
			Title: "各业务收入怎么变", ChartType: model.ChartColumn, Reason: "每个分部一列。之后新季度写进新行，这张图跟着变长",
			Source: model.SourceRule, Sheet: "利润表",
		})
	}
	if len(ex.Columns) == 0 && hasAny(ex, "iphone_revenue", "services_revenue") {
		out = append(out, model.ChartSpec{
			Title: "硬件还是服务：本季收入从哪来", ChartType: model.ChartPie, Reason: "硬件卖设备，服务把利润做厚。饼图看这一季收入从哪来",
			Source: model.SourceRule, Sheet: "利润表",
		})
	}
	if hasAny(ex, "total_revenue", "net_income") {
		out = append(out, model.ChartSpec{
			Title: "收入能留下多少利润", ChartType: model.ChartColumn, Reason: "营收和净利润放在一起，看这一季收入能留下多少",
			Source: model.SourceRule, Sheet: "利润表",
		})
	}
	if len(ex.Columns) == 0 && hasAny(ex, "services_revenue", "total_revenue") {
		out = append(out, model.ChartSpec{
			Title: "服务占收入的比例", ChartType: model.ChartColumn, Reason: "服务占比按财季排开，看商业模式有没有往高毛利移动",
			Source: model.SourceRule, Sheet: "利润表",
		})
	}
	if hasAny(ex, "cash", "long_term_debt", "equity") {
		out = append(out,
			model.ChartSpec{
				Title: "家底和债务谁更大", ChartType: model.ChartColumn, Reason: "现金证券、有息债务和股东权益放在一起，看家底盖不盖得住债",
				Source: model.SourceRule, Sheet: "资产负债表",
			},
			model.ChartSpec{
				Title: "净现金盖不盖得住债", ChartType: model.ChartColumn, Reason: "净现金和有息债务按财季排开，看健康度是变厚还是变薄",
				Source: model.SourceRule, Sheet: "资产负债表",
			},
		)
	}
	if hasAny(ex, "inventory", "receivables") {
		out = append(out, model.ChartSpec{
			Title: "存货和应收有没有胀", ChartType: model.ChartColumn, Reason: "存货和应收账款突然变大，是这季资产负债表上要盯的点",
			Source: model.SourceRule, Sheet: "资产负债表",
		})
	}
	if hasAny(ex, "operating_cash_flow", "free_cash_flow", "capital_return") {
		out = append(out,
			model.ChartSpec{
				Title: "这一季现金怎么来、怎么走", ChartType: model.ChartColumn, Reason: "经营现金流、资本开支、自由现金流和资本返还放在一起，看现金的来路和去向",
				Source: model.SourceRule, Sheet: "现金流量表",
			},
			model.ChartSpec{
				Title: "赚到的现金有没有还给股东", ChartType: model.ChartColumn, Reason: "经营现金流和资本返还按财季对比，看赚到的现金有没有分出去",
				Source: model.SourceRule, Sheet: "现金流量表",
			},
		)
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

func configuredSegments(ex model.Extraction) int {
	n := 0
	for key := range ex.Columns {
		if _, ok := ex.Number(key); ok {
			n++
		}
	}
	return n
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
