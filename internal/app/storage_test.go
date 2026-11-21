package app

import (
	"context"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/config"
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
