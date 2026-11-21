package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func f64(v float64) *float64 { return &v }

func TestPostgresUpsertIdempotent(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIGRATIONS_DIR", filepath.Join(root, "migrations"))
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.MigrationsDir = filepath.Join(root, "migrations")

	ex := model.Extraction{
		Ticker: "AAPL",
		Period: model.Period{FiscalYear: 2026, FiscalQuarter: 4},
		Facts: []model.Fact{
			{LineItem: "total_revenue", Value: f64(119.6), Unit: model.UnitBillionUSD},
			{LineItem: "net_income", Value: f64(24.8), Unit: model.UnitBillionUSD},
		},
	}
	n, err := s.Upsert(ctx, "wb-test", ex)
	if err != nil || n != 2 {
		t.Fatalf("upsert n=%d err=%v", n, err)
	}
	n2, err := s.Upsert(ctx, "wb-test", ex)
	if err != nil || n2 != 2 {
		t.Fatalf("re-upsert n=%d err=%v", n2, err)
	}
	rows, err := s.ListFacts(ctx, "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected >=2 facts, got %d", len(rows))
	}

	mem := &sheet.Memory{}
	d := &sheet.Dual{Sheets: mem, PG: s, Mode: sheet.ModeDual, Fail: sheet.FailAll}
	if _, err := d.Upsert(ctx, "wb-test", ex); err != nil {
		t.Fatal(err)
	}
	if len(mem.Facts) == 0 {
		t.Fatal("dual should write memory side")
	}
}
