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
金额统一换算成十亿美元（billion_usd）。毛利率和同比用小数（17% 写成 0.17）。
无法确定的字段省略，不要猜 0。
JSON 形状：
{"fiscal_year":2026,"fiscal_quarter":4,"facts":[{"line_item":"total_revenue","value":114.48,"unit":"billion_usd"}]}
line_item 只用这些键：total_revenue, iphone_revenue, mac_revenue, ipad_revenue, wearables_revenue, services_revenue, gross_margin, net_income, diluted_eps, iphone_units, iphone_yoy, mac_yoy, ipad_yoy, wearables_yoy, services_yoy, cash, short_term_investments, receivables, inventory, other_current_assets, long_term_investments, ppe_net, other_noncurrent_assets, accounts_payable, short_term_debt, deferred_revenue, other_current_liabilities, long_term_debt, other_noncurrent_liabilities, equity, operating_cash_flow, capex, free_cash_flow, capital_return, dividend_per_share, depreciation, other_operating, other_investing, other_financing, note`

const takeawaySystem = `你是买方研究员。根据下面的财季数字，用简体中文写 3 到 5 条最值得关注的变化。每条不超过 40 个字。只输出 JSON：{"takeaway":["..."]}`

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
	if len(text) > 60000 {
		text = text[:60000]
	}
	user := fmt.Sprintf("公司 %s (%s)，财年截止月 %d。若文中能判断财年财季，以文中为准。提示期间：%s\n\n%s",
		company.Name, company.Ticker, company.FiscalYearEndMonth, hint.Label(), text)
	raw, usage, err := e.Model.Complete(ctx, extractSystem, user)
	if err != nil {
		return model.Extraction{}, usage, err
	}
	ex, err := decodeModel(StripFence(raw), company.Ticker)
	return ex, usage, err
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
	var parsed struct {
		FiscalYear    int `json:"fiscal_year"`
		FiscalQuarter int `json:"fiscal_quarter"`
		Facts         []struct {
			LineItem string   `json:"line_item"`
			Value    *float64 `json:"value"`
			Text     string   `json:"text"`
			Unit     string   `json:"unit"`
		} `json:"facts"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return model.Extraction{}, fmt.Errorf("model json: %w", err)
	}
	ex := model.Extraction{
		Ticker: ticker,
		Period: model.Period{FiscalYear: parsed.FiscalYear, FiscalQuarter: parsed.FiscalQuarter},
	}
	for _, f := range parsed.Facts {
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
	if !ex.Period.Valid() || len(ex.Facts) == 0 {
		return model.Extraction{}, fmt.Errorf("model extraction missing period or facts")
	}
	return ex, nil
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
