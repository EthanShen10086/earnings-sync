package job

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/store/postgres"
)

func TestPGStoreIdempotentAcrossOpens(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mig := filepath.Join(root, "migrations")
	t.Setenv("MIGRATIONS_DIR", mig)

	pg, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	pg.MigrationsDir = mig
	if err := pg.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	a := NewPG(pg.DB())
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	hash := "pg-hash-" + suffix
	loc := "https://example.test/pg-" + suffix
	j := a.Create("AAPL", hash, loc, "url", false)
	if j.Status == model.StatusFailed {
		t.Fatalf("create failed: %s", j.Error)
	}
	j.Status = model.StatusSucceeded
	j.Cells = 3
	if err := a.Save(j); err != nil {
		t.Fatal(err)
	}

	// Simulate another process: new store handle on same DB.
	b := NewPG(pg.DB())
	got, ok := b.ByHash(hash)
	if !ok || got.ID != j.ID || got.Cells != 3 {
		t.Fatalf("cross-handle hash lookup %+v ok=%v", got, ok)
	}
	got, ok = b.ByLocator(loc)
	if !ok || got.Ticker != "AAPL" {
		t.Fatalf("cross-handle locator %+v", got)
	}
	got, ok = b.Get(j.ID)
	if !ok || got.DocHash != hash {
		t.Fatalf("get %+v", got)
	}
}
