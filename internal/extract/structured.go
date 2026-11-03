package extract

import (
	"encoding/csv"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// TryStructured reads a long-form csv of fiscal_year,fiscal_quarter,line_item,value,unit.
func TryStructured(text, filename string) (model.Extraction, bool) {
	if !strings.Contains(strings.ToLower(filename+" "+text[:min(200, len(text))]), "line_item") &&
		!strings.HasSuffix(strings.ToLower(filename), ".csv") {
		return model.Extraction{}, false
	}
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		return model.Extraction{}, false
	}
	header := map[string]int{}
	for i, h := range rows[0] {
		header[strings.ToLower(strings.TrimSpace(h))] = i
	}
	itemCol, okItem := header["line_item"]
	valueCol, okValue := header["value"]
	if !okItem || !okValue {
		return model.Extraction{}, false
	}
	var ex model.Extraction
	for _, row := range rows[1:] {
		if itemCol >= len(row) || valueCol >= len(row) {
			continue
		}
		item := normalizeItem(row[itemCol])
		if item == "" {
			continue
		}
		if col, ok := header["fiscal_year"]; ok && col < len(row) && ex.Period.FiscalYear == 0 {
			ex.Period.FiscalYear, _ = strconv.Atoi(strings.TrimSpace(row[col]))
		}
		if col, ok := header["fiscal_quarter"]; ok && col < len(row) && ex.Period.FiscalQuarter == 0 {
			ex.Period.FiscalQuarter, _ = strconv.Atoi(strings.TrimSpace(row[col]))
		}
		unit := model.UnitBillionUSD
		if col, ok := header["unit"]; ok && col < len(row) && strings.TrimSpace(row[col]) != "" {
			unit = strings.TrimSpace(row[col])
		}
		raw := strings.TrimSpace(row[valueCol])
		fact := model.Fact{LineItem: item, Unit: unit, Statement: statementOf(item)}
		if unit == model.UnitText {
			fact.Text = raw
		} else if n, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", ""), 64); err == nil {
			n = scale(item, n, unit)
			fact.Value = &n
			fact.Unit = storedUnit(item, unit)
		} else {
			fact.Text = raw
			fact.Unit = model.UnitText
		}
		ex.Facts = append(ex.Facts, fact)
	}
	if !ex.Period.Valid() || len(ex.Facts) == 0 {
		return model.Extraction{}, false
	}
	return ex, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeItem(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	switch s {
	case "revenue", "net_sales", "total_net_sales":
		return "total_revenue"
	case "iphone", "iphone_net_sales":
		return "iphone_revenue"
	case "mac":
		return "mac_revenue"
	case "ipad":
		return "ipad_revenue"
	case "services":
		return "services_revenue"
	case "wearables", "wearables_home_and_accessories":
		return "wearables_revenue"
	case "gross_margin_pct", "gross_margin_percent":
		return "gross_margin"
	case "eps", "diluted_eps_usd":
		return "diluted_eps"
	case "ocf", "cash_from_operations":
		return "operating_cash_flow"
	case "capital_expenditure", "purchases_of_ppe":
		return "capex"
	case "fcf":
		return "free_cash_flow"
	default:
		return s
	}
}

func statementOf(item string) string {
	switch item {
	case "cash", "short_term_investments", "receivables", "inventory", "other_current_assets",
		"long_term_investments", "ppe_net", "other_noncurrent_assets", "accounts_payable",
		"short_term_debt", "deferred_revenue", "other_current_liabilities", "long_term_debt",
		"other_noncurrent_liabilities", "equity":
		return model.StatementBalance
	case "operating_cash_flow", "capex", "free_cash_flow", "capital_return", "dividend_per_share",
		"depreciation", "other_operating", "other_investing", "other_financing":
		return model.StatementCashflow
	default:
		return model.StatementIncome
	}
}

func scale(item string, n float64, unit string) float64 {
	switch item {
	case "gross_margin", "iphone_yoy", "mac_yoy", "ipad_yoy", "wearables_yoy", "services_yoy":
		if n > 1.5 || n < -1.5 {
			return n / 100
		}
		return n
	case "diluted_eps", "dividend_per_share", "iphone_units":
		return n
	}
	switch unit {
	case model.UnitMillionUSD:
		return n / 1000
	case model.UnitUSD:
		return n / 1e9
	default:
		return n
	}
}

func storedUnit(item, unit string) string {
	switch item {
	case "gross_margin", "iphone_yoy", "mac_yoy", "ipad_yoy", "wearables_yoy", "services_yoy":
		return model.UnitRatio
	case "diluted_eps", "dividend_per_share":
		return model.UnitPerShare
	case "iphone_units":
		return model.UnitMillionUnits
	default:
		if unit == model.UnitText {
			return unit
		}
		return model.UnitBillionUSD
	}
}
