package extract

import (
	"encoding/csv"
	"sort"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// TryStructured reads a long-form csv. When several quarters are present, it returns the latest one.
func TryStructured(text, filename string) (model.Extraction, bool) {
	all, ok := TryStructuredAll(text, filename)
	if !ok {
		return model.Extraction{}, false
	}
	return all[len(all)-1], true
}

// TryStructuredAll groups a long-form csv into one extraction per fiscal quarter, oldest first.
func TryStructuredAll(text, filename string) ([]model.Extraction, bool) {
	if !strings.Contains(strings.ToLower(filename+" "+text[:min(200, len(text))]), "line_item") &&
		!strings.HasSuffix(strings.ToLower(filename), ".csv") {
		return nil, false
	}
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, false
	}
	header := map[string]int{}
	for i, h := range rows[0] {
		header[strings.ToLower(strings.TrimSpace(h))] = i
	}
	itemCol, okItem := header["line_item"]
	valueCol, okValue := header["value"]
	if !okItem || !okValue {
		return nil, false
	}
	type key struct{ year, quarter int }
	order := []key{}
	grouped := map[key]*model.Extraction{}
	for _, row := range rows[1:] {
		if itemCol >= len(row) || valueCol >= len(row) {
			continue
		}
		item := normalizeItem(row[itemCol])
		if item == "" {
			continue
		}
		k := key{}
		if col, ok := header["fiscal_year"]; ok && col < len(row) {
			k.year, _ = strconv.Atoi(strings.TrimSpace(row[col]))
		}
		if col, ok := header["fiscal_quarter"]; ok && col < len(row) {
			k.quarter, _ = strconv.Atoi(strings.TrimSpace(row[col]))
		}
		ex := grouped[k]
		if ex == nil {
			ex = &model.Extraction{Period: model.Period{FiscalYear: k.year, FiscalQuarter: k.quarter}}
			grouped[k] = ex
			order = append(order, k)
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
	if len(order) == 0 {
		return nil, false
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.year != b.year {
			return a.year < b.year
		}
		return a.quarter < b.quarter
	})
	out := make([]model.Extraction, 0, len(order))
	for _, k := range order {
		ex := grouped[k]
		if ex.Period.Valid() && len(ex.Facts) > 0 {
			out = append(out, *ex)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
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
	case "ocf", "cash_from_operations", "cash_and_cash_equivalents":
		if s == "cash_and_cash_equivalents" {
			return "cash"
		}
		return "operating_cash_flow"
	case "cash":
		return "cash"
	case "marketable_securities_current", "short_term_marketable_securities":
		return "short_term_investments"
	case "accounts_receivable", "accounts_receivable_net":
		return "receivables"
	case "inventories":
		return "inventory"
	case "marketable_securities_noncurrent", "long_term_marketable_securities":
		return "long_term_investments"
	case "property_plant_and_equipment_net", "ppe":
		return "ppe_net"
	case "shareholders_equity", "stockholders_equity", "total_shareholders_equity":
		return "equity"
	case "commercial_paper_and_current_term_debt":
		return "short_term_debt"
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
		if n >= 1e6 || n <= -1e6 {
			return n / 1e9
		}
		if n >= 1000 || n <= -1000 {
			return n / 1000
		}
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
