package sheet

import (
	"context"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// FactRow is one persisted earnings line for API/UI readback.
type FactRow struct {
	IdempotencyKey string   `json:"idempotency_key"`
	SpreadsheetID  string   `json:"spreadsheet_id"`
	Ticker         string   `json:"ticker"`
	FiscalYear     int      `json:"fiscal_year"`
	FiscalQuarter  int      `json:"fiscal_quarter"`
	Statement      string   `json:"statement"`
	LineItem       string   `json:"line_item"`
	Value          *float64 `json:"value,omitempty"`
	Text           string   `json:"text,omitempty"`
	Unit           string   `json:"unit,omitempty"`
}

// Reader lists stored facts. Postgres is the primary read path; Memory implements it for offline demos.
type Reader interface {
	ListFacts(ctx context.Context, ticker string) ([]FactRow, error)
}

// ReaderFrom returns Reader if store implements it (including Dual preferring PG).
func ReaderFrom(store Store) Reader {
	if store == nil {
		return nil
	}
	if d, ok := store.(*Dual); ok {
		if r, ok := d.PG.(Reader); ok {
			return r
		}
		if r, ok := d.Sheets.(Reader); ok {
			return r
		}
		return nil
	}
	if r, ok := store.(Reader); ok {
		return r
	}
	return nil
}

// FactRowsFromExtraction flattens an extraction into FactRows (shared by Memory and Postgres).
func FactRowsFromExtraction(spreadsheetID string, ex model.Extraction) []FactRow {
	out := make([]FactRow, 0, len(ex.Facts))
	for _, f := range ex.Facts {
		if f.LineItem == "" {
			continue
		}
		if f.Value == nil && f.Text == "" {
			continue
		}
		row := FactRow{
			IdempotencyKey: FactKey(spreadsheetID, ex.Ticker, ex.Period, f.LineItem),
			SpreadsheetID:  spreadsheetID,
			Ticker:         ex.Ticker,
			FiscalYear:     ex.Period.FiscalYear,
			FiscalQuarter:  ex.Period.FiscalQuarter,
			Statement:      f.Statement,
			LineItem:       f.LineItem,
			Value:          f.Value,
			Text:           f.Text,
			Unit:           f.Unit,
		}
		out = append(out, row)
	}
	return out
}
