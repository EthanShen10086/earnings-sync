package orchestrator

import (
	"context"
	"fmt"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
	"github.com/EthanShen10086/earnings-sync/internal/store/postgres"
)

func buildStoreLocal(settings config.Settings, mem *sheet.Memory) (sheet.Store, func() error, error) {
	cleanup := func() error { return nil }
	mode := sheet.ParseWriteMode(settings.StorageMode)
	fail := sheet.ParseFailPolicy(settings.StorageFail)
	var pgStore sheet.Store
	if settings.DatabaseURL != "" {
		pg, err := postgres.Open(context.Background(), settings.DatabaseURL)
		if err != nil {
			return nil, cleanup, err
		}
		if settings.MigrationsDir != "" {
			pg.MigrationsDir = settings.MigrationsDir
		}
		pgStore = pg
		cleanup = pg.Close
	}
	switch mode {
	case sheet.ModeSheetsOnly:
		return mem, cleanup, nil
	case sheet.ModePGOnly:
		if pgStore == nil {
			_ = cleanup()
			return nil, func() error { return nil }, fmt.Errorf("STORAGE_MODE=pg-only requires DATABASE_URL")
		}
		return &sheet.Dual{Sheets: mem, PG: pgStore, Mode: mode, Fail: fail}, cleanup, nil
	case sheet.ModeDual:
		if pgStore == nil {
			_ = cleanup()
			return nil, func() error { return nil }, fmt.Errorf("STORAGE_MODE=dual requires DATABASE_URL")
		}
		return &sheet.Dual{Sheets: mem, PG: pgStore, Mode: mode, Fail: fail}, cleanup, nil
	default:
		_ = cleanup()
		return nil, func() error { return nil }, fmt.Errorf("unknown STORAGE_MODE %q", settings.StorageMode)
	}
}
