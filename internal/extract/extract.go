package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/model"
)

const extractSystem = `你是财报结构化抽取器。只输出 JSON，不要解释。
阅读英文财报正文，抽出最近一个财季的三张表。金额用十亿美元（billion_usd），毛利率和同比用小数（17% 写成 0.17）。
利润表和分部收入只用「最近三个月 / Three Months Ended」那一列，不要用十二个月累计，也不要用上年同期列。
资产负债表只用最近一个报告日那一列。
现金流量表如果只有十二个月累计、没有单季列，就不要填写现金流字段，不要把全年数写进这个财季。
短期借款 short_term_debt = 商业票据 + 一年内到期的长期债务。
other_noncurrent_assets = 无形资产 + 其他非流动资产。
无法确定的字段省略，不要猜 0。
同时用简体中文写 analysis，6 到 8 条，每条不超过 80 字。利润表、现金流量表、资产负债表都要写到。
依次覆盖：利润表里异常好的、异常坏的、业务结构（分部收入不要当成公司总营收）、怎么赚钱、现金流量表、资产负债表健康度、公司质地。只根据正文，不要编造。
JSON 形状：
{"periods":[{"fiscal_year":2026,"fiscal_quarter":4,"facts":[{"line_item":"total_revenue","value":114.48,"unit":"billion_usd"},{"line_item":"cash","value":36.31,"unit":"billion_usd"}]}],"analysis":["好：……","坏：……","结构：……","赚钱：……","负债表：……","质地：……"]}
line_item 只用用户消息里列出的键。`

const takeawaySystem = `你是买方研究员。根据下面已经换算好的财季数字，用简体中文写 6 条解读，每条不超过 70 字。
依次覆盖：这季异常好的一点、异常坏的一点、收入结构、公司靠什么赚钱、资产负债表健康度、公司质地。
只根据给定数字，不要编造。只输出 JSON：{"takeaway":["..."]}`

// Extractor turns a document into facts. Structured files skip the model; prose uses it.
type Extractor struct {
	Model Provider
}

func (e Extractor) FromDocument(ctx context.Context, doc model.Document, company config.Company, hint model.Period) (model.Extraction, TokenUsage, error) {
	if ex, ok := TryStructured(doc.Text, doc.Filename); ok {
		ex.Ticker = company.Ticker
		return ex, TokenUsage{}, nil
	}
	if e.Model == nil {
		return model.Extraction{}, TokenUsage{}, fmt.Errorf("document is not a structured table and no model is configured")
	}
	text := doc.Text
	if len(text) > 24000 {
		text = text[:24000]
	}
	user := fmt.Sprintf("公司 %s (%s)，财年截止月 %d。财季以正文的季度截止日和这个截止月为准，不要被文件发布日期带偏。提示期间：%s\n允许的 line_item：%s\n\n%s",
		company.Name, company.Ticker, company.FiscalYearEndMonth, hint.Label(), company.AllowedLineItems(), text)
	raw, usage, err := e.Model.Complete(ctx, extractSystem, user)
	if err != nil {
		return model.Extraction{}, usage, err
	}
	ex, err := decodeModel(StripFence(raw), company.Ticker)
	return ex, usage, err
}

// ParseReport reads an English filing once and returns every extracted quarter plus the Chinese reading.
func (e Extractor) ParseReport(ctx context.Context, doc model.Document, company config.Company, hint model.Period) ([]model.Extraction, []string, TokenUsage, error) {
	if e.Model == nil {
		return nil, nil, TokenUsage{}, fmt.Errorf("document is not a structured table and no model is configured")
	}
	text := doc.Text
	if len(text) > 24000 {
		text = text[:24000]
	}
	user := fmt.Sprintf("公司 %s (%s)，财年截止月 %d。财季以正文的季度截止日和这个截止月为准。提示期间：%s\n允许的 line_item：%s\n\n%s",
		company.Name, company.Ticker, company.FiscalYearEndMonth, hint.Label(), company.AllowedLineItems(), text)
	raw, usage, err := e.Model.Complete(ctx, extractSystem, user)
	if err != nil {
		return nil, nil, usage, err
	}
	periods, analysis, err := decodeReport(StripFence(raw), company.Ticker)
	return periods, analysis, usage, err
}

// MissingCore lists balance-sheet and income fields a real filing parse must contain.
func MissingCore(ex model.Extraction) []string {
	need := []string{"total_revenue", "net_income", "cash", "receivables", "inventory", "equity", "accounts_payable", "long_term_debt"}
	var miss []string
	for _, item := range need {
		if _, ok := ex.Number(item); !ok {
			miss = append(miss, item)
		}
	}
	return miss
}

func (e Extractor) Takeaway(ctx context.Context, ex model.Extraction, warnings []string) ([]string, TokenUsage, error) {
	if e.Model == nil {
		return fallbackTakeaway(ex, warnings), TokenUsage{}, nil
	}
	b, _ := json.Marshal(map[string]any{"period": ex.Period.Label(), "facts": ex.Facts, "warnings": warnings})
	raw, usage, err := e.Model.Complete(ctx, takeawaySystem, string(b))
	if err != nil {
		return fallbackTakeaway(ex, warnings), usage, nil
	}
	var parsed struct {
		Takeaway []string `json:"takeaway"`
	}
	if err := json.Unmarshal([]byte(StripFence(raw)), &parsed); err != nil || len(parsed.Takeaway) == 0 {
		return fallbackTakeaway(ex, warnings), usage, nil
	}
	return parsed.Takeaway, usage, nil
}

func decodeModel(raw, ticker string) (model.Extraction, error) {
	periods, _, err := decodeReport(raw, ticker)
	if err != nil {
		return model.Extraction{}, err
	}
	return periods[len(periods)-1], nil
}

func decodeReport(raw, ticker string) ([]model.Extraction, []string, error) {
	var parsed struct {
		Periods []struct {
			FiscalYear    int          `json:"fiscal_year"`
			FiscalQuarter int          `json:"fiscal_quarter"`
			Facts         []parsedFact `json:"facts"`
		} `json:"periods"`
		FiscalYear    int          `json:"fiscal_year"`
		FiscalQuarter int          `json:"fiscal_quarter"`
		Facts         []parsedFact `json:"facts"`
		Analysis      []string     `json:"analysis"`
		Takeaway      []string     `json:"takeaway"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, nil, fmt.Errorf("model json: %w", err)
	}
	type one struct {
		year, quarter int
		facts         []parsedFact
	}
	groups := make([]one, 0, len(parsed.Periods))
	if len(parsed.Periods) == 0 && (parsed.FiscalYear != 0 || len(parsed.Facts) > 0) {
		groups = append(groups, one{parsed.FiscalYear, parsed.FiscalQuarter, parsed.Facts})
	}
	for _, p := range parsed.Periods {
		groups = append(groups, one{p.FiscalYear, p.FiscalQuarter, p.Facts})
	}
	var out []model.Extraction
	for _, g := range groups {
		ex := model.Extraction{Ticker: ticker, Period: model.Period{FiscalYear: g.year, FiscalQuarter: g.quarter}}
		for _, f := range g.facts {
			item := normalizeItem(f.LineItem)
			if item == "" {
				continue
			}
			unit := f.Unit
			if unit == "" {
				unit = model.UnitBillionUSD
			}
			fact := model.Fact{LineItem: item, Unit: unit, Statement: statementOf(item), Text: f.Text}
			if f.Value != nil {
				n := scale(item, *f.Value, unit)
				fact.Value = &n
				fact.Unit = storedUnit(item, unit)
			}
			ex.Facts = append(ex.Facts, fact)
		}
		if ex.Period.Valid() && len(ex.Facts) > 0 {
			fixGrossMargin(&ex)
			out = append(out, ex)
		}
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("model extraction missing period or facts")
	}
	analysis := parsed.Analysis
	if len(analysis) == 0 {
		analysis = parsed.Takeaway
	}
	return out, analysis, nil
}

// fixGrossMargin corrects the common mix-up of gross profit dollars with the margin ratio.
// 54.82 billion of gross profit was being stored as 54.82% .
func fixGrossMargin(ex *model.Extraction) {
	gm, okGM := ex.Number("gross_margin")
	rev, okRev := ex.Number("total_revenue")
	if !okGM || !okRev || rev <= 0 || gm <= 0.52 {
		return
	}
	ratio := (gm * 100) / rev
	if ratio <= 0.3 || ratio >= 0.52 {
		return
	}
	for i := range ex.Facts {
		if ex.Facts[i].LineItem == "gross_margin" && ex.Facts[i].Value != nil {
			ex.Facts[i].Value = &ratio
			ex.Facts[i].Unit = model.UnitRatio
		}
	}
}

type parsedFact struct {
	LineItem string   `json:"line_item"`
	Value    *float64 `json:"value"`
	Text     string   `json:"text"`
	Unit     string   `json:"unit"`
}

func fallbackTakeaway(ex model.Extraction, warnings []string) []string {
	var lines []string
	if v, ok := ex.Number("total_revenue"); ok {
		lines = append(lines, fmt.Sprintf("%s 总营收 %.2f 十亿美元", ex.Period.Label(), v))
	}
	if v, ok := ex.Number("iphone_revenue"); ok {
		lines = append(lines, fmt.Sprintf("iPhone 营收 %.2f 十亿美元", v))
	}
	if v, ok := ex.Number("services_revenue"); ok {
		lines = append(lines, fmt.Sprintf("服务营收 %.2f 十亿美元", v))
	}
	if v, ok := ex.Number("net_income"); ok {
		lines = append(lines, fmt.Sprintf("净利润 %.2f 十亿美元", v))
	}
	if v, ok := ex.Number("free_cash_flow"); ok {
		lines = append(lines, fmt.Sprintf("自由现金流 %.2f 十亿美元", v))
	}
	if len(lines) > 4 {
		lines = lines[:4]
	}
	if len(warnings) > 0 {
		lines = append(lines, "勾稽："+warnings[0])
	}
	if len(lines) == 0 {
		lines = append(lines, ex.Period.Label()+" 已入库，关注空白的资产负债项目")
	}
	return lines
}

// ChartAdvice asks the model for extra chart ideas. Failure returns no extras.
func (e Extractor) ChartAdvice(ctx context.Context, ex model.Extraction) ([]model.ChartSpec, error) {
	if e.Model == nil {
		return nil, nil
	}
	b, _ := json.Marshal(ex.Facts)
	raw, _, err := e.Model.Complete(ctx, `只输出 JSON：{"charts":[{"title":"","chart_type":"COLUMN","reason":""}]}。chart_type 只能是 COLUMN 或 PIE。不要重复「总营收按期间的柱状图」这种显而易见的图。没有补充就返回空数组。`, string(b))
	if err != nil {
		return nil, nil
	}
	var parsed struct {
		Charts []model.ChartSpec `json:"charts"`
	}
	if err := json.Unmarshal([]byte(StripFence(raw)), &parsed); err != nil {
		return nil, nil
	}
	var out []model.ChartSpec
	for _, c := range parsed.Charts {
		c.ChartType = strings.ToUpper(c.ChartType)
		if c.Title == "" || (c.ChartType != model.ChartColumn && c.ChartType != model.ChartPie) {
			continue
		}
		c.Source = model.SourceModel
		out = append(out, c)
	}
	return out, nil
}
