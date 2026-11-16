package sheet

import (
	"context"
	"sync"

	"github.com/EthanShen10086/earnings-sync/internal/macro"
	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Write is one input cell the pipeline asked to store.
type Write struct {
	Sheet string
	A1    string
	Value any
}

// Store writes facts and charts. Tests use Memory; production uses Google.
type Store interface {
	Upsert(ctx context.Context, spreadsheetID string, ex model.Extraction) (int, error)
	UpsertAll(ctx context.Context, spreadsheetID string, periods []model.Extraction) (int, error)
	ApplyCharts(ctx context.Context, spreadsheetID string, specs []model.ChartSpec) (int, error)
	UpsertMacro(ctx context.Context, spreadsheetID string, rel model.MacroRelease) (int, error)
}

// Memory records writes for tests.
type Memory struct {
	mu     sync.Mutex
	Writes []Write
	Charts []model.ChartSpec
	Titles map[string]bool
	Err    error
}

func (m *Memory) Upsert(_ context.Context, _ string, ex model.Extraction) (int, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, f := range ex.Facts {
		ref, ok := locate(f.LineItem, ex)
		if !ok {
			continue
		}
		var v any
		if f.Value != nil {
			v = *f.Value
		} else if f.Text != "" {
			v = f.Text
		} else {
			continue
		}
		m.Writes = append(m.Writes, Write{Sheet: ref.Sheet, A1: ref.Col + "5", Value: v})
		n++
	}
	return n, nil
}

func (m *Memory) UpsertAll(ctx context.Context, spreadsheetID string, periods []model.Extraction) (int, error) {
	n := 0
	for _, ex := range periods {
		wrote, err := m.Upsert(ctx, spreadsheetID, ex)
		n += wrote
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (m *Memory) UpsertMacro(_ context.Context, _ string, rel model.MacroRelease) (int, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	panels := rel.Panels
	if len(panels) == 0 {
		panels = []model.MacroPanel{{ID: "monthly", Sheet: macro.SheetName, Layout: "rate", Title: rel.Title}}
	}
	for _, panel := range panels {
		switch panel.Layout {
		case "level":
			if !wantLevel(rel, panel) {
				continue
			}
			grid := macro.RenderLevel(panel, nil, rel.Levels)
			for _, cell := range grid.Inputs {
				m.Writes = append(m.Writes, Write{Sheet: panel.Sheet, A1: macro.A1(cell.Col, cell.Row), Value: cell.Value})
			}
			for _, mark := range grid.Marks {
				if mark.Kind != "yoy_pct" && mark.Kind != "vs_peak_pct" && mark.Kind != "yoy_delta" && mark.Kind != "vs_peak_delta" {
					continue
				}
				if mark.Row < len(grid.Values) && len(grid.Values[mark.Row]) > 1 {
					m.Writes = append(m.Writes, Write{Sheet: panel.Sheet, A1: macro.A1(1, mark.Row), Value: grid.Values[mark.Row][1]})
				}
			}
			for _, ch := range grid.Charts {
				m.Charts = append(m.Charts, model.ChartSpec{Title: ch.Title, ChartType: model.ChartColumn, Sheet: panel.Sheet, Source: model.SourceRule, Reason: "历年同期水平"})
			}
			n += len(grid.Inputs)
		default:
			if !wantRate(rel) {
				continue
			}
			title := panel.Title
			if title == "" {
				title = rel.Title
			}
			grid := macro.Render(title, nil, rel.Points)
			name := panel.Sheet
			if name == "" {
				name = macro.SheetName
			}
			for _, cell := range grid.Cells {
				m.Writes = append(m.Writes, Write{Sheet: name, A1: macro.A1(cell.Col, cell.Row), Value: cell.Value})
			}
			n += len(grid.Cells)
		}
	}
	return n, nil
}

func (m *Memory) ApplyCharts(_ context.Context, _ string, specs []model.ChartSpec) (int, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Titles == nil {
		m.Titles = map[string]bool{}
	}
	added := 0
	for _, spec := range specs {
		if spec.Source != model.SourceRule {
			continue
		}
		if _, ok := templateByTitle(spec.Title); !ok {
			continue
		}
		if m.Titles[spec.Title] {
			continue
		}
		m.Titles[spec.Title] = true
		m.Charts = append(m.Charts, spec)
		added++
	}
	return added, nil
}
