package acquire

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// FlowFact is one SEC company-concept observation, in USD.
type FlowFact struct {
	Start time.Time
	End   time.Time
	Val   float64
}

// QuarterAmount isolates one fiscal quarter from year-to-date XBRL flows.
// Q4 is the fiscal year minus the nine-month figure that shares the same start date.
func QuarterAmount(facts []FlowFact, fiscalYear, fiscalQuarter, fiscalYearEndMonth int) (float64, bool) {
	endYear, endMonth := quarterEnd(fiscalYear, fiscalQuarter, fiscalYearEndMonth)
	var current *FlowFact
	for i := range facts {
		f := &facts[i]
		if f.End.Year() != endYear || int(f.End.Month()) != endMonth {
			continue
		}
		if current == nil || f.End.Sub(f.Start) > current.End.Sub(current.Start) {
			current = f
		}
	}
	if current == nil {
		return 0, false
	}
	if fiscalQuarter == 1 || current.End.Sub(current.Start) < 120*24*time.Hour {
		return current.Val, true
	}
	prevYear, prevMonth := quarterEnd(fiscalYear, fiscalQuarter-1, fiscalYearEndMonth)
	var prev *FlowFact
	for i := range facts {
		f := &facts[i]
		if f.End.Year() != prevYear || int(f.End.Month()) != prevMonth {
			continue
		}
		if !sameDay(f.Start, current.Start) {
			continue
		}
		if prev == nil || f.End.Sub(f.Start) > prev.End.Sub(prev.Start) {
			prev = f
		}
	}
	if prev == nil {
		return 0, false
	}
	return current.Val - prev.Val, true
}

func quarterEnd(fiscalYear, fiscalQuarter, fiscalYearEndMonth int) (year, month int) {
	month = fiscalYearEndMonth - (4-fiscalQuarter)*3
	year = fiscalYear
	for month <= 0 {
		month += 12
		year--
	}
	return year, month
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// SupplementCash fills quarterly cash-flow facts the earnings release only states as a full year.
func SupplementCash(ctx context.Context, client *http.Client, cik, userAgent string, fiscalYearEndMonth int, ex *model.Extraction) error {
	if ex == nil || !ex.Period.Valid() {
		return nil
	}
	if _, ok := ex.Number("operating_cash_flow"); ok {
		if _, ok := ex.Number("capex"); ok {
			return nil
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	cik = strings.TrimLeft(cik, "0")
	padded := cik
	for len(padded) < 10 {
		padded = "0" + padded
	}
	load := func(concept string) ([]FlowFact, error) {
		url := "https://data.sec.gov/api/xbrl/companyconcept/CIK" + padded + "/us-gaap/" + concept + ".json"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		if err != nil {
			return nil, err
		}
		if res.StatusCode >= 300 {
			return nil, fmt.Errorf("sec concept %s: %s", concept, res.Status)
		}
		return decodeFlows(body)
	}
	set := func(item, concept string, flip bool) {
		if _, ok := ex.Number(item); ok {
			return
		}
		facts, err := load(concept)
		if err != nil {
			return
		}
		val, ok := QuarterAmount(facts, ex.Period.FiscalYear, ex.Period.FiscalQuarter, fiscalYearEndMonth)
		if !ok {
			return
		}
		if flip && val < 0 {
			val = -val
		}
		billions := val / 1e9
		ex.Facts = append(ex.Facts, model.Fact{
			Statement: model.StatementCashflow,
			LineItem:  item,
			Value:     &billions,
			Unit:      model.UnitBillionUSD,
		})
	}
	set("operating_cash_flow", "NetCashProvidedByUsedInOperatingActivities", false)
	set("capex", "PaymentsToAcquirePropertyPlantAndEquipment", true)
	divFacts, divErr := load("PaymentsOfDividends")
	buyFacts, buyErr := load("PaymentsForRepurchaseOfCommonStock")
	if _, ok := ex.Number("capital_return"); !ok && divErr == nil && buyErr == nil {
		div, divOK := QuarterAmount(divFacts, ex.Period.FiscalYear, ex.Period.FiscalQuarter, fiscalYearEndMonth)
		buy, buyOK := QuarterAmount(buyFacts, ex.Period.FiscalYear, ex.Period.FiscalQuarter, fiscalYearEndMonth)
		if divOK && buyOK {
			billions := (abs(div) + abs(buy)) / 1e9
			ex.Facts = append(ex.Facts, model.Fact{
				Statement: model.StatementCashflow,
				LineItem:  "capital_return",
				Value:     &billions,
				Unit:      model.UnitBillionUSD,
			})
		}
	}
	if _, ok := ex.Number("free_cash_flow"); !ok {
		ocf, ocfOK := ex.Number("operating_cash_flow")
		capex, capexOK := ex.Number("capex")
		if ocfOK && capexOK {
			fcf := ocf - capex
			ex.Facts = append(ex.Facts, model.Fact{
				Statement: model.StatementCashflow,
				LineItem:  "free_cash_flow",
				Value:     &fcf,
				Unit:      model.UnitBillionUSD,
			})
		}
	}
	return nil
}

func abs(n float64) float64 {
	if n < 0 {
		return -n
	}
	return n
}

func decodeFlows(body []byte) ([]FlowFact, error) {
	var parsed struct {
		Units struct {
			USD []struct {
				Start string  `json:"start"`
				End   string  `json:"end"`
				Val   float64 `json:"val"`
			} `json:"USD"`
		} `json:"units"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var out []FlowFact
	for _, row := range parsed.Units.USD {
		start, err1 := time.Parse("2006-01-02", row.Start)
		end, err2 := time.Parse("2006-01-02", row.End)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, FlowFact{Start: start, End: end, Val: row.Val})
	}
	return out, nil
}
