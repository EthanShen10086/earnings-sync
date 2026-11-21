package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/macro"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store persists extractions in PostgreSQL and implements sheet.Store + sheet.Reader.
// Each UpsertAll / UpsertMacro / ApplyCharts runs in a single SQL transaction.
type Store struct {
	db            *sql.DB
	MigrationsDir string
}

// Open connects with DATABASE_URL (or the given DSN) and applies migrations.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	s := &Store{db: db, MigrationsDir: os.Getenv("MIGRATIONS_DIR")}
	if err := s.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrationsDir() (string, error) {
	if s.MigrationsDir != "" {
		return s.MigrationsDir, nil
	}
	candidates := []string{
		"migrations",
		filepath.Join("..", "migrations"),
		filepath.Join("..", "..", "migrations"),
		filepath.Join("..", "..", "..", "migrations"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			abs, err := filepath.Abs(c)
			if err != nil {
				return c, nil
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("migrations directory not found (set MIGRATIONS_DIR)")
}

// Migrate applies SQL files under migrations/ in name order, once each.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return err
	}
	dir, err := s.migrationsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Upsert(ctx context.Context, spreadsheetID string, ex model.Extraction) (int, error) {
	return s.UpsertAll(ctx, spreadsheetID, []model.Extraction{ex})
}

func (s *Store) UpsertAll(ctx context.Context, spreadsheetID string, periods []model.Extraction) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	n := 0
	for _, ex := range periods {
		if !ex.Period.Valid() {
			return 0, fmt.Errorf("period is invalid")
		}
		for _, row := range sheet.FactRowsFromExtraction(spreadsheetID, ex) {
			_, err := tx.ExecContext(ctx, `
INSERT INTO earnings_facts (
  idempotency_key, spreadsheet_id, ticker, fiscal_year, fiscal_quarter,
  statement, line_item, value, text_value, unit, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, now())
ON CONFLICT (idempotency_key) DO UPDATE SET
  value = EXCLUDED.value,
  text_value = EXCLUDED.text_value,
  unit = EXCLUDED.unit,
  statement = EXCLUDED.statement,
  spreadsheet_id = EXCLUDED.spreadsheet_id,
  updated_at = now()`,
				row.IdempotencyKey, row.SpreadsheetID, row.Ticker, row.FiscalYear, row.FiscalQuarter,
				row.Statement, row.LineItem, nullFloat(row.Value), row.Text, row.Unit,
			)
			if err != nil {
				return n, err
			}
			n++
		}
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

func (s *Store) ApplyCharts(ctx context.Context, spreadsheetID string, specs []model.ChartSpec) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	added := 0
	for _, spec := range specs {
		if spec.Source != model.SourceRule || spec.Title == "" {
			continue
		}
		key := sheet.ChartKey(spreadsheetID, spec.Title)
		res, err := tx.ExecContext(ctx, `
INSERT INTO earnings_charts (
  idempotency_key, spreadsheet_id, title, chart_type, sheet_name, source, reason
) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (idempotency_key) DO NOTHING`,
			key, spreadsheetID, spec.Title, spec.ChartType, spec.Sheet, spec.Source, spec.Reason,
		)
		if err != nil {
			return added, err
		}
		ra, _ := res.RowsAffected()
		if ra > 0 {
			added++
		}
	}
	if err := tx.Commit(); err != nil {
		return added, err
	}
	return added, nil
}

func (s *Store) UpsertMacro(ctx context.Context, spreadsheetID string, rel model.MacroRelease) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	n := 0
	panels := rel.Panels
	if len(panels) == 0 {
		panels = []model.MacroPanel{{ID: "monthly", Sheet: macro.SheetName, Layout: "rate", Title: rel.Title}}
	}
	for _, panel := range panels {
		switch panel.Layout {
		case "level":
			for _, pt := range rel.Levels {
				if panel.Window != "" && pt.Window != "" && panel.Window != pt.Window {
					continue
				}
				if pt.Indicator == "" || pt.Value == nil {
					continue
				}
				key := sheet.MacroKey(spreadsheetID, panel.ID, fmt.Sprintf("%d", pt.Year), pt.Window, pt.Indicator)
				_, err := tx.ExecContext(ctx, `
INSERT INTO macro_points (
  idempotency_key, spreadsheet_id, panel_id, sheet_name, window_name, year_value,
  indicator, value, layout, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'level', now())
ON CONFLICT (idempotency_key) DO UPDATE SET
  value = EXCLUDED.value, updated_at = now()`,
					key, spreadsheetID, panel.ID, panel.Sheet, pt.Window, pt.Year, pt.Indicator, *pt.Value,
				)
				if err != nil {
					return n, err
				}
				n++
			}
		default:
			for _, pt := range rel.Points {
				if pt.Indicator == "" || pt.Value == nil {
					continue
				}
				key := sheet.MacroKey(spreadsheetID, panel.ID, pt.Period, pt.Basis, pt.Indicator)
				_, err := tx.ExecContext(ctx, `
INSERT INTO macro_points (
  idempotency_key, spreadsheet_id, panel_id, sheet_name, period, basis,
  indicator, value, layout, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'rate', now())
ON CONFLICT (idempotency_key) DO UPDATE SET
  value = EXCLUDED.value, updated_at = now()`,
					key, spreadsheetID, panel.ID, panel.Sheet, pt.Period, pt.Basis, pt.Indicator, *pt.Value,
				)
				if err != nil {
					return n, err
				}
				n++
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

func (s *Store) ListFacts(ctx context.Context, ticker string) ([]sheet.FactRow, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	rows, err := s.db.QueryContext(ctx, `
SELECT idempotency_key, spreadsheet_id, ticker, fiscal_year, fiscal_quarter,
       statement, line_item, value, text_value, unit
FROM earnings_facts
WHERE ($1 = '' OR ticker = $1)
ORDER BY ticker, fiscal_year, fiscal_quarter, line_item`, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sheet.FactRow
	for rows.Next() {
		var r sheet.FactRow
		var val sql.NullFloat64
		if err := rows.Scan(&r.IdempotencyKey, &r.SpreadsheetID, &r.Ticker, &r.FiscalYear, &r.FiscalQuarter,
			&r.Statement, &r.LineItem, &val, &r.Text, &r.Unit); err != nil {
			return nil, err
		}
		if val.Valid {
			v := val.Float64
			r.Value = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
