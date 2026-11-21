package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/features"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
	"github.com/EthanShen10086/earnings-sync/internal/store/postgres"
)

// StoreBuild describes how storage was wired, including soft degradations.
type StoreBuild struct {
	Store      sheet.Store
	Cleanup    func() error
	Mode       sheet.WriteMode // effective mode after degradation
	WantMode   sheet.WriteMode
	SheetsKind string // "google" | "memory"
	PGKind     string // "postgres" | "memory" | "none"
	Notes      []string
}

// BuildStore wires Sheets (Google or Memory) and optional Postgres into a Dual writer.
// Without Google credentials, Sheets side is Memory so local/CI can still exercise dual.
// Missing / failing advanced backends degrade instead of aborting the minimum path,
// except pg-only with no usable store at all (then Memory stands in).
func BuildStore(ctx context.Context, settings config.Settings) (store sheet.Store, cleanup func() error, err error) {
	b, err := BuildStoreDetailed(ctx, settings, features.Bundle{})
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return b.Store, b.Cleanup, nil
}

// BuildStoreDetailed is BuildStore plus a degradation report. feat controls whether
// Google Sheets is attempted; zero Bundle means auto (creds present → try Google).
func BuildStoreDetailed(ctx context.Context, settings config.Settings, feat features.Bundle) (StoreBuild, error) {
	out := StoreBuild{
		Cleanup:  func() error { return nil },
		WantMode: sheet.ParseWriteMode(settings.StorageMode),
		Mode:     sheet.ParseWriteMode(settings.StorageMode),
		PGKind:   "none",
	}
	fail := sheet.ParseFailPolicy(settings.StorageFail)

	tryGoogle := settings.GoogleCreds != ""
	if feat.Flags.SheetsGoogle != "" {
		tryGoogle = feat.UseGoogleSheets()
	}

	var sheetsStore sheet.Store
	if tryGoogle {
		g, err := sheet.NewGoogle(ctx, settings.GoogleCreds)
		if err != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("google sheets unavailable (%v); degraded to Memory", err))
			slog.Warn("google sheets unavailable; degraded to Memory", "err", err.Error())
		} else {
			sheetsStore = g
			out.SheetsKind = "google"
		}
	}
	if sheetsStore == nil {
		sheetsStore = &sheet.Memory{}
		if out.SheetsKind == "" {
			out.SheetsKind = "memory"
		}
		if settings.GoogleCreds == "" || !tryGoogle {
			out.Notes = append(out.Notes, "sheets side using Memory (no Google credentials or FEATURE_SHEETS_GOOGLE off)")
		}
	}

	var pgStore sheet.Store
	needsPG := out.WantMode == sheet.ModePGOnly || out.WantMode == sheet.ModeDual
	if settings.DatabaseURL != "" {
		pg, err := postgres.Open(ctx, settings.DatabaseURL)
		if err != nil {
			msg := fmt.Sprintf("postgres unavailable (%v)", err)
			if needsPG {
				out.Notes = append(out.Notes, msg+"; degraded away from "+string(out.WantMode))
				slog.Warn("postgres unavailable; degrading storage mode", "want", out.WantMode, "err", err.Error())
			} else {
				out.Notes = append(out.Notes, msg+"; ignored for sheets-only")
			}
		} else {
			if settings.MigrationsDir != "" {
				pg.MigrationsDir = settings.MigrationsDir
			}
			pgStore = pg
			out.PGKind = "postgres"
			out.Cleanup = pg.Close
		}
	} else if needsPG {
		out.Notes = append(out.Notes, "DATABASE_URL unset; degraded away from "+string(out.WantMode))
		slog.Warn("DATABASE_URL unset; degrading storage mode", "want", out.WantMode)
	}

	// Soft-degrade modes that required PG but have none.
	if needsPG && pgStore == nil {
		// Keep data: Memory sheets path always available.
		out.Mode = sheet.ModeSheetsOnly
		out.Notes = append(out.Notes, "effective mode=sheets-only (Memory) so ingest→query still works")
	}

	switch out.Mode {
	case sheet.ModeSheetsOnly:
		if pgStore == nil {
			out.Store = sheetsStore
			return out, nil
		}
		out.Store = &sheet.Dual{Sheets: sheetsStore, PG: pgStore, Mode: out.Mode, Fail: fail}
		return out, nil
	case sheet.ModePGOnly:
		out.Store = &sheet.Dual{Sheets: sheetsStore, PG: pgStore, Mode: out.Mode, Fail: fail}
		return out, nil
	case sheet.ModeDual:
		out.Store = &sheet.Dual{Sheets: sheetsStore, PG: pgStore, Mode: out.Mode, Fail: fail}
		return out, nil
	default:
		_ = out.Cleanup()
		return StoreBuild{Cleanup: func() error { return nil }}, fmt.Errorf("unknown STORAGE_MODE %q", settings.StorageMode)
	}
}
