package orm_test

import (
	"context"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/orm"
)

func TestMemoryCRUDAndTenantScope(t *testing.T) {
	db := orm.Memory()
	ctx := context.Background()

	s := db.WithContext(ctx).Tenant("acme")
	if err := s.Create("facts", orm.Row{
		"ticker": "AAPL",
		"value":  1.0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Tenant("beta").Create("facts", orm.Row{
		"ticker": "MSFT",
		"value":  2.0,
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := db.WithContext(ctx).Tenant("acme").Query("facts").Find()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["ticker"] != "AAPL" {
		t.Fatalf("tenant scope: %+v", rows)
	}

	n, err := db.WithContext(ctx).Tenant("acme").Query("facts").Where("ticker", "AAPL").Updates(orm.Row{"value": 9.0})
	if err != nil || n != 1 {
		t.Fatalf("updates n=%d err=%v", n, err)
	}
	got, err := db.WithContext(ctx).Tenant("acme").Query("facts").Where("ticker", "AAPL").First()
	if err != nil {
		t.Fatal(err)
	}
	if got["value"] != 9.0 {
		t.Fatalf("value=%v", got["value"])
	}

	deleted, err := db.WithContext(ctx).Tenant("beta").Query("facts").Where("ticker", "MSFT").Delete()
	if err != nil || deleted != 1 {
		t.Fatalf("delete n=%d err=%v", deleted, err)
	}
	if db.Backend() != "memory" {
		t.Fatalf("backend=%s", db.Backend())
	}
}

func TestTransactionMemory(t *testing.T) {
	db := orm.Memory()
	err := db.Transaction(context.Background(), func(s *orm.Session) error {
		return s.Create("jobs", orm.Row{"name": "x"})
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := db.WithContext(context.Background()).Query("jobs").Count()
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}
