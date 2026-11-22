package lakehouse_test

import (
	"context"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/lakehouse"
	"github.com/EthanShen10086/earnings-sync/internal/orm"
)

func TestPartitionAndExportStub(t *testing.T) {
	db := orm.Memory()
	store := lakehouse.NewStore(db)
	ctx := context.Background()
	et := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)

	key, n, err := store.AppendFacts(ctx, []lakehouse.Fact{
		{TenantID: "acme", Ticker: "AAPL", Period: "2026Q4", LineItem: "revenue", Value: 100, EventTime: et},
		{TenantID: "acme", Ticker: "AAPL", Period: "2026Q4", LineItem: "net_income", Value: 20, EventTime: et},
	})
	if err != nil || n != 2 {
		t.Fatalf("append n=%d err=%v", n, err)
	}
	if key.String() != "acme/2026/11" {
		t.Fatalf("key=%s", key.String())
	}
	facts := store.ListPartition(key)
	if len(facts) != 2 {
		t.Fatalf("facts=%d", len(facts))
	}
	rows, err := db.WithContext(ctx).Tenant("acme").Query("lakehouse_facts").Find()
	if err != nil || len(rows) != 2 {
		t.Fatalf("orm mirror=%d err=%v", len(rows), err)
	}

	job, err := store.StartExport("exp-1", key, "stub://warehouse")
	if err != nil || job.Status != lakehouse.ExportPending {
		t.Fatalf("start: %+v err=%v", job, err)
	}
	job, err = store.RunExport("exp-1")
	if err != nil || job.Status != lakehouse.ExportSucceeded || job.Rows != 2 {
		t.Fatalf("run: %+v err=%v", job, err)
	}
}
