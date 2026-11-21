package app

import (
	"context"
	"fmt"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
	"github.com/EthanShen10086/earnings-sync/internal/store/postgres"
)

// BuildStore wires Sheets (Google or Memory) and optional Postgres into a Dual writer.
// Without Google credentials, Sheets side is Memory so local/CI can still exercise dual.
func BuildStore(ctx context.Context, settings config.Settings) (store sheet.Store, cleanup func() error, err error) {
	cleanup = func() error { return nil }
	mode := sheet.ParseWriteMode(settings.StorageMode)
	fail := sheet.ParseFailPolicy(settings.StorageFail)

	var sheetsStore sheet.Store
	if settings.GoogleCreds != "" {
		g, err := sheet.NewGoogle(ctx, settings.GoogleCreds)
		if err != nil {
			return nil, cleanup, fmt.Errorf("google sheets: %w", err)
		}
		sheetsStore = g
	}

	var pgStore sheet.Store
	var pg *postgres.Store
	needsPG := mode == sheet.ModePGOnly || mode == sheet.ModeDual
	if needsPG || settings.DatabaseURL != "" {
		if settings.DatabaseURL == "" && needsPG {
			return nil, cleanup, fmt.Errorf("STORAGE_MODE=%s requires DATABASE_URL", mode)
		}
		if settings.DatabaseURL != "" {
			pg, err = postgres.Open(ctx, settings.DatabaseURL)
			if err != nil {
				return nil, cleanup, err
			}
			if settings.MigrationsDir != "" {
				pg.MigrationsDir = settings.MigrationsDir
			}
			pgStore = pg
			cleanup = pg.Close
		}
	}

	needsSheets := mode == sheet.ModeSheetsOnly || mode == sheet.ModeDual
	if needsSheets && sheetsStore == nil {
		// No Google key: Memory stands in so dual/sheets-only still runs offline.
		sheetsStore = &sheet.Memory{}
	}

	switch mode {
	case sheet.ModeSheetsOnly:
		if settings.DatabaseURL == "" {
			return sheetsStore, cleanup, nil
		}
		// PG connected but mode sheets-only: still wrap so ops can flip mode later via same Dual shape.
		return &sheet.Dual{Sheets: sheetsStore, PG: pgStore, Mode: mode, Fail: fail}, cleanup, nil
	case sheet.ModePGOnly:
		return &sheet.Dual{Sheets: sheetsStore, PG: pgStore, Mode: mode, Fail: fail}, cleanup, nil
	case sheet.ModeDual:
		return &sheet.Dual{Sheets: sheetsStore, PG: pgStore, Mode: mode, Fail: fail}, cleanup, nil
	default:
		_ = cleanup()
		return nil, func() error { return nil }, fmt.Errorf("unknown STORAGE_MODE %q", settings.StorageMode)
	}
}
