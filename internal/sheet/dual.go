package sheet

import (
	"context"
	"fmt"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// WriteMode selects which backends receive pipeline writes.
type WriteMode string

const (
	ModeSheetsOnly WriteMode = "sheets-only"
	ModePGOnly     WriteMode = "pg-only"
	ModeDual       WriteMode = "dual"
)

// FailPolicy controls dual-write error handling.
type FailPolicy string

const (
	// FailAll fails the call if any selected backend fails (default).
	FailAll FailPolicy = "all"
	// FailPartial writes every selected backend; returns an error if any failed,
	// but does not roll back siblings (Sheets has no shared TX with PG).
	FailPartial FailPolicy = "partial"
)

// ParseWriteMode normalizes env values.
func ParseWriteMode(s string) WriteMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(ModeSheetsOnly), "sheets", "sheet":
		return ModeSheetsOnly
	case string(ModePGOnly), "pg", "postgres", "postgresql":
		return ModePGOnly
	case string(ModeDual), "both":
		return ModeDual
	default:
		return WriteMode(strings.ToLower(strings.TrimSpace(s)))
	}
}

// ParseFailPolicy normalizes env values.
func ParseFailPolicy(s string) FailPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(FailAll), "fail-all", "strict":
		return FailAll
	case string(FailPartial), "best-effort", "continue":
		return FailPartial
	default:
		return FailPolicy(strings.ToLower(strings.TrimSpace(s)))
	}
}

// Dual fans writes out to Sheets and/or Postgres according to Mode.
// Transaction boundary: each backend commits independently; Dual does not
// provide a distributed transaction. Prefer FailAll so a partial dual write
// surfaces as an error to the caller/job.
type Dual struct {
	Sheets Store
	PG     Store
	Mode   WriteMode
	Fail   FailPolicy
}

func (d *Dual) targets() (sheets, pg bool, err error) {
	mode := d.Mode
	if mode == "" {
		mode = ModeSheetsOnly
	}
	switch mode {
	case ModeSheetsOnly:
		if d.Sheets == nil {
			return false, false, fmt.Errorf("storage mode %s requires a Sheets store", mode)
		}
		return true, false, nil
	case ModePGOnly:
		if d.PG == nil {
			return false, false, fmt.Errorf("storage mode %s requires a Postgres store", mode)
		}
		return false, true, nil
	case ModeDual:
		if d.Sheets == nil || d.PG == nil {
			return false, false, fmt.Errorf("storage mode dual requires both Sheets and Postgres stores")
		}
		return true, true, nil
	default:
		return false, false, fmt.Errorf("unknown storage mode %q (want sheets-only|pg-only|dual)", mode)
	}
}

type writeFn func(ctx context.Context, s Store) (int, error)

func (d *Dual) run(ctx context.Context, op string, fn writeFn) (int, error) {
	useSheets, usePG, err := d.targets()
	if err != nil {
		return 0, err
	}
	policy := d.Fail
	if policy == "" {
		policy = FailAll
	}
	total := 0
	var errs []string
	type step struct {
		name  string
		store Store
	}
	var steps []step
	if useSheets {
		steps = append(steps, step{name: "sheets", store: d.Sheets})
	}
	if usePG {
		steps = append(steps, step{name: "postgres", store: d.PG})
	}
	for _, st := range steps {
		n, err := fn(ctx, st.store)
		total += n
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", st.name, err))
			if policy == FailAll {
				return total, fmt.Errorf("%s via %s: %w", op, st.name, err)
			}
		}
	}
	if len(errs) > 0 {
		return total, fmt.Errorf("%s partial failures: %s", op, strings.Join(errs, "; "))
	}
	return total, nil
}

func (d *Dual) Upsert(ctx context.Context, spreadsheetID string, ex model.Extraction) (int, error) {
	return d.run(ctx, "Upsert", func(ctx context.Context, s Store) (int, error) {
		return s.Upsert(ctx, spreadsheetID, ex)
	})
}

func (d *Dual) UpsertAll(ctx context.Context, spreadsheetID string, periods []model.Extraction) (int, error) {
	return d.run(ctx, "UpsertAll", func(ctx context.Context, s Store) (int, error) {
		return s.UpsertAll(ctx, spreadsheetID, periods)
	})
}

func (d *Dual) ApplyCharts(ctx context.Context, spreadsheetID string, specs []model.ChartSpec) (int, error) {
	return d.run(ctx, "ApplyCharts", func(ctx context.Context, s Store) (int, error) {
		return s.ApplyCharts(ctx, spreadsheetID, specs)
	})
}

func (d *Dual) UpsertMacro(ctx context.Context, spreadsheetID string, rel model.MacroRelease) (int, error) {
	return d.run(ctx, "UpsertMacro", func(ctx context.Context, s Store) (int, error) {
		return s.UpsertMacro(ctx, spreadsheetID, rel)
	})
}

// AsGoogle unwraps a Google backend from Store or Dual (Sheets side).
func AsGoogle(s Store) *Google {
	switch v := s.(type) {
	case *Google:
		return v
	case *Dual:
		if v == nil {
			return nil
		}
		if g, ok := v.Sheets.(*Google); ok {
			return g
		}
	}
	return nil
}

// FactKey builds the stable idempotency key for one reported line item.
func FactKey(spreadsheetID, ticker string, p model.Period, lineItem string) string {
	return fmt.Sprintf("fact|%s|%s|%d|%d|%s", spreadsheetID, strings.ToUpper(ticker), p.FiscalYear, p.FiscalQuarter, lineItem)
}

// ChartKey builds the idempotency key for a chart title in a workbook.
func ChartKey(spreadsheetID, title string) string {
	return fmt.Sprintf("chart|%s|%s", spreadsheetID, title)
}

// MacroKey builds the idempotency key for one macro input cell/point.
func MacroKey(spreadsheetID, panel, period, basis, indicator string) string {
	return fmt.Sprintf("macro|%s|%s|%s|%s|%s", spreadsheetID, panel, period, basis, indicator)
}
