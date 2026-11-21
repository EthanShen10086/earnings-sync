package orchestrator

import (
	"context"
	"log/slog"

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
			slog.Warn("orchestrator: postgres unavailable; degrading storage", "want", mode, "err", err.Error())
		} else {
			if settings.MigrationsDir != "" {
				pg.MigrationsDir = settings.MigrationsDir
			}
			pgStore = pg
			cleanup = pg.Close
		}
	}
	needsPG := mode == sheet.ModePGOnly || mode == sheet.ModeDual
	if needsPG && pgStore == nil {
		slog.Warn("orchestrator: DATABASE_URL missing or PG down; using Memory sheets-only", "want", mode)
		return mem, cleanup, nil
	}
	switch mode {
	case sheet.ModeSheetsOnly:
		return mem, cleanup, nil
	case sheet.ModePGOnly, sheet.ModeDual:
		return &sheet.Dual{Sheets: mem, PG: pgStore, Mode: mode, Fail: fail}, cleanup, nil
	default:
		_ = cleanup()
		slog.Warn("orchestrator: unknown STORAGE_MODE; using Memory", "mode", settings.StorageMode)
		return mem, func() error { return nil }, nil
	}
}
