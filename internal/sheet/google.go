package sheet

import (
	"context"
	"fmt"
	"strconv"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const firstDataRow = 2
const lastDataRow = 80

// Google updates only the yellow input cells of the quarterly template.
type Google struct {
	svc *sheets.Service
}

func NewGoogle(ctx context.Context, credFile string) (*Google, error) {
	if credFile == "" {
		return nil, fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS is empty")
	}
	svc, err := sheets.NewService(ctx, option.WithCredentialsFile(credFile))
	if err != nil {
		return nil, err
	}
	return &Google{svc: svc}, nil
}

func (g *Google) Upsert(ctx context.Context, spreadsheetID string, ex model.Extraction) (int, error) {
	if spreadsheetID == "" {
		return 0, fmt.Errorf("spreadsheet id is empty")
	}
	if !ex.Period.Valid() {
		return 0, fmt.Errorf("period is invalid")
	}
	row, err := g.ensureRow(ctx, spreadsheetID, ex.Period)
	if err != nil {
		return 0, err
	}
	var data []*sheets.ValueRange
	for _, f := range ex.Facts {
		ref, ok := inputColumns[f.LineItem]
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
		data = append(data, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!%s%d", ref.Sheet, ref.Col, row),
			Values: [][]any{{v}},
		})
	}
	if len(data) == 0 {
		return 0, nil
	}
	_, err = g.svc.Spreadsheets.Values.BatchUpdate(spreadsheetID, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
		Data:             data,
	}).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func (g *Google) ensureRow(ctx context.Context, spreadsheetID string, period model.Period) (int, error) {
	resp, err := g.svc.Spreadsheets.Values.Get(spreadsheetID, "利润表!A2:B80").
		ValueRenderOption("UNFORMATTED_VALUE").Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	empty := 0
	for i, row := range resp.Values {
		sheetRow := firstDataRow + i
		if len(row) == 0 || row[0] == nil || row[0] == "" {
			if empty == 0 {
				empty = sheetRow
			}
			continue
		}
		year, yok := asInt(row[0])
		quarter := 0
		qok := false
		if len(row) > 1 {
			quarter, qok = asInt(row[1])
		}
		if yok && qok && year == period.FiscalYear && quarter == period.FiscalQuarter {
			return sheetRow, nil
		}
	}
	if empty == 0 {
		if len(resp.Values) < lastDataRow-1 {
			empty = firstDataRow + len(resp.Values)
		}
	}
	if empty == 0 {
		return 0, fmt.Errorf("period grid is full; extend the template past row %d", lastDataRow)
	}
	_, err = g.svc.Spreadsheets.Values.Update(spreadsheetID, fmt.Sprintf("利润表!A%d:B%d", empty, empty), &sheets.ValueRange{
		Values: [][]any{{period.FiscalYear, period.FiscalQuarter}},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	// Keep the same period key on the other two statements.
	_, err = g.svc.Spreadsheets.Values.BatchUpdate(spreadsheetID, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
		Data: []*sheets.ValueRange{
			{Range: fmt.Sprintf("资产负债表!A%d:B%d", empty, empty), Values: [][]any{{period.FiscalYear, period.FiscalQuarter}}},
			{Range: fmt.Sprintf("现金流量表!A%d:B%d", empty, empty), Values: [][]any{{period.FiscalYear, period.FiscalQuarter}}},
		},
	}).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	return empty, nil
}

func (g *Google) ApplyCharts(ctx context.Context, spreadsheetID string, specs []model.ChartSpec) (int, error) {
	if spreadsheetID == "" {
		return 0, fmt.Errorf("spreadsheet id is empty")
	}
	ss, err := g.svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets.properties,sheets.charts").Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	sheetIDs := map[string]int64{}
	existing := map[string]bool{}
	for _, sh := range ss.Sheets {
		if sh.Properties != nil {
			sheetIDs[sh.Properties.Title] = sh.Properties.SheetId
		}
		for _, ch := range sh.Charts {
			if ch.Spec != nil && ch.Spec.Title != "" {
				existing[ch.Spec.Title] = true
			}
		}
	}
	var reqs []*sheets.Request
	for _, spec := range specs {
		if spec.Source != "" && spec.Source != model.SourceRule {
			continue
		}
		tmpl, ok := templateByTitle(spec.Title)
		if !ok || existing[spec.Title] {
			continue
		}
		sid, ok := sheetIDs[tmpl.Sheet]
		if !ok {
			continue
		}
		reqs = append(reqs, &sheets.Request{AddChart: addChart(sid, tmpl)})
	}
	if len(reqs) == 0 {
		return 0, nil
	}
	_, err = g.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: reqs}).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	return len(reqs), nil
}

func addChart(sheetID int64, tmpl tmplChart) *sheets.AddChartRequest {
	domain := grid(sheetID, tmpl.HeaderRow, tmpl.EndRow, tmpl.DomainCol, tmpl.DomainCol+1)
	series := grid(sheetID, tmpl.HeaderRow, tmpl.EndRow, tmpl.SeriesCol, tmpl.SeriesCol+1)
	spec := &sheets.ChartSpec{Title: tmpl.Title}
	if tmpl.Type == model.ChartPie {
		spec.PieChart = &sheets.PieChartSpec{
			LegendPosition: "RIGHT_LEGEND",
			Domain:         &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{domain}}},
			Series:         &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{series}}},
		}
	} else {
		spec.BasicChart = &sheets.BasicChartSpec{
			ChartType:      "COLUMN",
			HeaderCount:    1,
			LegendPosition: "NO_LEGEND",
			Domains: []*sheets.BasicChartDomain{{
				Domain: &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{domain}}},
			}},
			Series: []*sheets.BasicChartSeries{{
				Series:     &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{series}}},
				TargetAxis: "LEFT_AXIS",
			}},
		}
	}
	return &sheets.AddChartRequest{Chart: &sheets.EmbeddedChart{
		Spec: spec,
		Position: &sheets.EmbeddedObjectPosition{
			OverlayPosition: &sheets.OverlayPosition{
				AnchorCell: &sheets.GridCoordinate{SheetId: sheetID, RowIndex: tmpl.AnchorRow, ColumnIndex: tmpl.AnchorCol},
			},
		},
	}}
}

func grid(sheetID, startRow, endRow, startCol, endCol int64) *sheets.GridRange {
	return &sheets.GridRange{
		SheetId:          sheetID,
		StartRowIndex:    startRow,
		EndRowIndex:      endRow,
		StartColumnIndex: startCol,
		EndColumnIndex:   endCol,
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	default:
		return 0, false
	}
}
