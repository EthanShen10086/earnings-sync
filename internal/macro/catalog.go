package macro

import "strings"

const (
	// SheetName is the single macro grid. Rows are indicators, columns are months.
	SheetName = "宏观"

	BasisYTD = "ytd"
	BasisYoY = "yoy"
	BasisMoM = "mom"
)

// Indicator is one fixed row. The same key can appear under more than one basis.
type Indicator struct {
	Key    string
	Basis  string
	Label  string
	Format string // percent or number
}

// Catalog is the China macro table: 累计同比, then 当期同比, then 环比.
func Catalog() []Indicator {
	return []Indicator{
		{Key: "gdp", Basis: BasisYTD, Label: "GDP增速", Format: "percent"},
		{Key: "retail", Basis: BasisYTD, Label: "社会消费品零售总额", Format: "percent"},
		{Key: "fai", Basis: BasisYTD, Label: "固定资产投资总额", Format: "percent"},
		{Key: "fai_ex_property", Basis: BasisYTD, Label: "扣除房地产开发投资", Format: "percent"},
		{Key: "infra", Basis: BasisYTD, Label: "基础设施建设投资", Format: "percent"},
		{Key: "manufacturing", Basis: BasisYTD, Label: "制造业投资", Format: "percent"},
		{Key: "private", Basis: BasisYTD, Label: "民间投资", Format: "percent"},
		{Key: "private_ex_property", Basis: BasisYTD, Label: "扣除房地产开发投资", Format: "percent"},
		{Key: "property_investment", Basis: BasisYTD, Label: "房地产开发投资", Format: "percent"},
		{Key: "property_area", Basis: BasisYTD, Label: "全国商品房销售面积", Format: "percent"},
		{Key: "property_sales", Basis: BasisYTD, Label: "全国商品房销售额", Format: "percent"},
		{Key: "industrial", Basis: BasisYTD, Label: "规模以上工业增加值", Format: "percent"},
		{Key: "industrial_profit", Basis: BasisYTD, Label: "规模以上企业利润总额", Format: "percent"},
		{Key: "trade", Basis: BasisYTD, Label: "进出口金额（美元）", Format: "percent"},
		{Key: "exports", Basis: BasisYTD, Label: "出口金额（美元）", Format: "percent"},
		{Key: "imports", Basis: BasisYTD, Label: "进口金额（美元）", Format: "percent"},
		{Key: "cpi", Basis: BasisYTD, Label: "CPI", Format: "percent"},
		{Key: "ppi", Basis: BasisYTD, Label: "PPI", Format: "percent"},
		{Key: "cpi_ppi", Basis: BasisYTD, Label: "CPI-PPI", Format: "percent"},

		{Key: "gdp", Basis: BasisYoY, Label: "GDP增速（当季）", Format: "percent"},
		{Key: "retail", Basis: BasisYoY, Label: "社会消费品零售总额", Format: "percent"},
		{Key: "industrial", Basis: BasisYoY, Label: "工业增加值", Format: "percent"},
		{Key: "trade", Basis: BasisYoY, Label: "进出口金额（美元）", Format: "percent"},
		{Key: "exports", Basis: BasisYoY, Label: "出口金额（美元）", Format: "percent"},
		{Key: "imports", Basis: BasisYoY, Label: "进口金额（美元）", Format: "percent"},
		{Key: "cpi", Basis: BasisYoY, Label: "CPI", Format: "percent"},
		{Key: "ppi", Basis: BasisYoY, Label: "PPI", Format: "percent"},
		{Key: "cpi_ppi", Basis: BasisYoY, Label: "CPI-PPI", Format: "percent"},
		{Key: "pmi", Basis: BasisYoY, Label: "PMI", Format: "number"},

		{Key: "retail", Basis: BasisMoM, Label: "社会消费品零售总额环比（%）", Format: "percent"},
		{Key: "fai", Basis: BasisMoM, Label: "固定资产投资完成额环比（%）", Format: "percent"},
		{Key: "industrial", Basis: BasisMoM, Label: "规模以上工业增加值环比（%）", Format: "percent"},
	}
}

// Indicators returns the rows of one basis, in sheet order.
func Indicators(basis string) []Indicator {
	basis = NormalizeBasis(basis)
	var out []Indicator
	for _, ind := range Catalog() {
		if ind.Basis == basis {
			out = append(out, ind)
		}
	}
	return out
}

// BasisLabel is the section header written in column A.
func BasisLabel(basis string) string {
	switch NormalizeBasis(basis) {
	case BasisYTD:
		return "累计同比"
	case BasisYoY:
		return "当期同比"
	case BasisMoM:
		return "环比"
	default:
		return ""
	}
}

// NormalizeBasis accepts the sheet header or a short English key.
func NormalizeBasis(s string) string {
	switch fold(s) {
	case "ytd", "ytdyoy", "cumyoy", "累计同比", "累计":
		return BasisYTD
	case "yoy", "periodyoy", "当期同比", "当月同比", "当季同比", "同比":
		return BasisYoY
	case "mom", "环比", "当月环比":
		return BasisMoM
	default:
		return ""
	}
}

// Resolve picks the catalog row for a basis. occurrence distinguishes the two
// 「扣除房地产开发投资」 rows when the name is repeated down the sheet.
func Resolve(basis, raw string, occurrence int) (Indicator, bool) {
	basis = NormalizeBasis(basis)
	raw = strings.TrimSpace(raw)
	if basis == "" || raw == "" {
		return Indicator{}, false
	}
	key := canonical(raw)
	var hits []Indicator
	for _, ind := range Indicators(basis) {
		if ind.Key == key || ind.Label == raw {
			hits = append(hits, ind)
		}
	}
	if len(hits) == 1 {
		return hits[0], true
	}
	if len(hits) > 1 && occurrence >= 0 && occurrence < len(hits) {
		return hits[occurrence], true
	}
	return Indicator{}, false
}

func canonical(raw string) string {
	if k, ok := aliases[fold(raw)]; ok {
		return k
	}
	return fold(raw)
}

func fold(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

var aliases = map[string]string{
	"gdp":         "gdp",
	"gdp增速":       "gdp",
	"国内生产总值":      "gdp",
	"gdp增速（当季）":   "gdp",
	"gdp当季":       "gdp",
	"社零":          "retail",
	"社会消费品零售总额":   "retail",
	"社会消费品零售总额环比": "retail",
	"社会消费品零售总额环比（%）": "retail",
	"固投":                "fai",
	"固定资产投资":            "fai",
	"固定资产投资总额":          "fai",
	"固定资产投资完成额":         "fai",
	"固定资产投资完成额环比":       "fai",
	"固定资产投资完成额环比（%）":    "fai",
	"固投扣除房地产开发投资":       "fai_ex_property",
	"扣除房地产开发投资（固投）":     "fai_ex_property",
	"faiexproperty":     "fai_ex_property",
	"基建":                "infra",
	"基础设施":              "infra",
	"基础设施建设投资":          "infra",
	"制造业":               "manufacturing",
	"制造业投资":             "manufacturing",
	"民间投资":              "private",
	"民间投资扣除房地产开发投资":     "private_ex_property",
	"扣除房地产开发投资（民间）":     "private_ex_property",
	"privateexproperty": "private_ex_property",
	"房地产开发投资":           "property_investment",
	"房地产投资":             "property_investment",
	"商品房销售面积":           "property_area",
	"全国商品房销售面积":         "property_area",
	"商品房销售额":            "property_sales",
	"全国商品房销售额":          "property_sales",
	"工业增加值":             "industrial",
	"规模以上工业增加值":         "industrial",
	"规模以上工业增加值环比":       "industrial",
	"规模以上工业增加值环比（%）":    "industrial",
	"工业企业利润":            "industrial_profit",
	"规模以上企业利润":          "industrial_profit",
	"规模以上企业利润总额":        "industrial_profit",
	"进出口":               "trade",
	"进出口金额":             "trade",
	"进出口金额（美元）":         "trade",
	"出口":                "exports",
	"出口金额":              "exports",
	"出口金额（美元）":          "exports",
	"进口":                "imports",
	"进口金额":              "imports",
	"进口金额（美元）":          "imports",
	"cpi":               "cpi",
	"ppi":               "ppi",
	"cpippi":            "cpi_ppi",
	"cpi-ppi":           "cpi_ppi",
	"cpippi差额":          "cpi_ppi",
	"pmi":               "pmi",
	"制造业pmi":            "pmi",
}
