package app

import (
	"context"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/features"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func TestBuildStoreSheetsOnlyMemory(t *testing.T) {
	store, cleanup, err := BuildStore(context.Background(), config.Settings{
		StorageMode: "sheets-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	mem, ok := store.(*sheet.Memory)
	if !ok {
		t.Fatalf("got %T", store)
	}
	v := 1.0
	_, err = store.Upsert(context.Background(), "s", model.Extraction{
		Ticker: "AAPL",
		Period: model.Period{FiscalYear: 2026, FiscalQuarter: 1},
		Facts:  []model.Fact{{LineItem: "total_revenue", Value: &v}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mem.Facts) == 0 {
		t.Fatal("expected memory facts")
	}
}

func TestBuildStoreDualWithoutDatabaseDegrades(t *testing.T) {
	built, err := BuildStoreDetailed(context.Background(), config.Settings{
		StorageMode: "dual",
		// no DATABASE_URL
	}, features.Bundle{})
	if err != nil {
		t.Fatal(err)
	}
	defer built.Cleanup()
	if built.Mode != sheet.ModeSheetsOnly {
		t.Fatalf("effective mode=%s want sheets-only", built.Mode)
	}
	if built.SheetsKind != "memory" {
		t.Fatalf("sheets=%s", built.SheetsKind)
	}
	v := 2.0
	if _, err := built.Store.Upsert(context.Background(), "s", model.Extraction{
		Ticker: "AAPL",
		Period: model.Period{FiscalYear: 2026, FiscalQuarter: 2},
		Facts:  []model.Fact{{LineItem: "net_income", Value: &v}},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := sheet.ReaderFrom(built.Store).ListFacts(context.Background(), "AAPL")
	if err != nil || len(rows) == 0 {
		t.Fatalf("query after degrade: rows=%d err=%v", len(rows), err)
	}
}

func TestBuildStoreSheetsFlagOffSkipsGoogle(t *testing.T) {
	feat := features.Resolve(features.Flags{SheetsGoogle: features.WantOff}, features.Deps{
		GoogleCreds: "/nonexistent/creds.json",
	})
	built, err := BuildStoreDetailed(context.Background(), config.Settings{
		StorageMode: "sheets-only",
		GoogleCreds: "/nonexistent/creds.json",
	}, feat)
	if err != nil {
		t.Fatal(err)
	}
	defer built.Cleanup()
	if built.SheetsKind != "memory" {
		t.Fatalf("expected memory when FEATURE_SHEETS_GOOGLE=off, got %s", built.SheetsKind)
	}
}
