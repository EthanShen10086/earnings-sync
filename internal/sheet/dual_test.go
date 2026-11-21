package sheet

import (
	"context"
	"errors"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

func f64(v float64) *float64 { return &v }

func sampleExtraction() model.Extraction {
	return model.Extraction{
		Ticker: "AAPL",
		Period: model.Period{FiscalYear: 2026, FiscalQuarter: 4},
		Facts: []model.Fact{
			{Statement: model.StatementIncome, LineItem: "total_revenue", Value: f64(119.6), Unit: model.UnitBillionUSD},
			{Statement: model.StatementIncome, LineItem: "net_income", Value: f64(24.8), Unit: model.UnitBillionUSD},
		},
	}
}

func TestDualModes(t *testing.T) {
	ex := sampleExtraction()
	ctx := context.Background()

	t.Run("sheets-only", func(t *testing.T) {
		a, b := &Memory{}, &Memory{}
		d := &Dual{Sheets: a, PG: b, Mode: ModeSheetsOnly}
		n, err := d.Upsert(ctx, "sheet-1", ex)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("expected writes")
		}
		if len(a.Facts) == 0 || len(b.Facts) != 0 {
			t.Fatalf("sheets=%d pg=%d", len(a.Facts), len(b.Facts))
		}
	})

	t.Run("pg-only", func(t *testing.T) {
		a, b := &Memory{}, &Memory{}
		d := &Dual{Sheets: a, PG: b, Mode: ModePGOnly}
		if _, err := d.Upsert(ctx, "sheet-1", ex); err != nil {
			t.Fatal(err)
		}
		if len(a.Facts) != 0 || len(b.Facts) == 0 {
			t.Fatalf("sheets=%d pg=%d", len(a.Facts), len(b.Facts))
		}
	})

	t.Run("dual", func(t *testing.T) {
		a, b := &Memory{}, &Memory{}
		d := &Dual{Sheets: a, PG: b, Mode: ModeDual}
		if _, err := d.Upsert(ctx, "sheet-1", ex); err != nil {
			t.Fatal(err)
		}
		if len(a.Facts) == 0 || len(b.Facts) == 0 {
			t.Fatalf("sheets=%d pg=%d", len(a.Facts), len(b.Facts))
		}
		rows, err := ReaderFrom(d).ListFacts(ctx, "AAPL")
		if err != nil || len(rows) == 0 {
			t.Fatalf("reader: %v rows=%d", err, len(rows))
		}
	})
}

func TestDualFailPolicies(t *testing.T) {
	ex := sampleExtraction()
	ctx := context.Background()
	boom := errors.New("boom")

	t.Run("fail-all stops after sheets error", func(t *testing.T) {
		a, b := &Memory{Err: boom}, &Memory{}
		d := &Dual{Sheets: a, PG: b, Mode: ModeDual, Fail: FailAll}
		_, err := d.Upsert(ctx, "sheet-1", ex)
		if err == nil {
			t.Fatal("expected error")
		}
		if len(b.Facts) != 0 {
			t.Fatalf("pg should not be written under fail-all after sheets error, got %d", len(b.Facts))
		}
	})

	t.Run("fail-partial continues to pg", func(t *testing.T) {
		a, b := &Memory{Err: boom}, &Memory{}
		d := &Dual{Sheets: a, PG: b, Mode: ModeDual, Fail: FailPartial}
		_, err := d.Upsert(ctx, "sheet-1", ex)
		if err == nil {
			t.Fatal("expected aggregate error")
		}
		if len(b.Facts) == 0 {
			t.Fatal("pg should still receive writes under fail-partial")
		}
	})
}

func TestDualIdempotentKeys(t *testing.T) {
	ex := sampleExtraction()
	mem := &Memory{}
	d := &Dual{Sheets: mem, Mode: ModeSheetsOnly}
	ctx := context.Background()
	if _, err := d.Upsert(ctx, "s1", ex); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Upsert(ctx, "s1", ex); err != nil {
		t.Fatal(err)
	}
	rows, _ := mem.ListFacts(ctx, "AAPL")
	if len(rows) != 2 {
		t.Fatalf("expected 2 unique facts after re-upsert, got %d", len(rows))
	}
}

func TestParseWriteMode(t *testing.T) {
	if ParseWriteMode("DUAL") != ModeDual {
		t.Fatal(ParseWriteMode("DUAL"))
	}
	if ParseFailPolicy("best-effort") != FailPartial {
		t.Fatal(ParseFailPolicy("best-effort"))
	}
}
