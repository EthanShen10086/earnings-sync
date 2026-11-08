package sheet

import (
	"context"
	"fmt"
	"strconv"

	"github.com/EthanShen10086/earnings-sync/internal/config"
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
	svc, err := sheets.NewService(ctx, option.WithCredentialsFile(credFile), option.WithScopes(
		sheets.SpreadsheetsScope,
		"https://www.googleapis.com/auth/drive.file",
	))
	if err != nil {
		return nil, err
	}
	return &Google{svc: svc}, nil
}

func (g *Google) Ping(ctx context.Context, spreadsheetID string) error {
	if spreadsheetID == "" {
		return fmt.Errorf("spreadsheet id 为空")
	}
	_, err := g.svc.Spreadsheets.Values.Get(spreadsheetID, "利润表!A1").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("读不到利润表表头：%w。把表格共享给服务账号 JSON 里的 client_email，权限选编辑者", err)
	}
	return nil
}

func (g *Google) Upsert(ctx context.Context, spreadsheetID string, ex model.Extraction) (int, error) {
	return g.UpsertAll(ctx, spreadsheetID, []model.Extraction{ex})
}

// UpsertAll writes every quarter in one read and one BatchUpdate.
func (g *Google) UpsertAll(ctx context.Context, spreadsheetID string, periods []model.Extraction) (int, error) {
	if spreadsheetID == "" {
		return 0, fmt.Errorf("这家公司还没有 spreadsheet_id。复制 Apple 那份表格模板，把新 id 写进 configs/companies.yaml")
	}
	resp, err := g.svc.Spreadsheets.Values.Get(spreadsheetID, "利润表!A2:B80").
		ValueRenderOption("UNFORMATTED_VALUE").Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	var data []*sheets.ValueRange
	cells := 0
	for _, ex := range periods {
		if !ex.Period.Valid() {
			return 0, fmt.Errorf("period is invalid")
		}
		row, fresh := placeRow(resp, ex.Period)
		if row == 0 {
			return 0, fmt.Errorf("period grid is full; extend the template past row %d", lastDataRow)
		}
		if fresh {
			keys := [][]any{{ex.Period.FiscalYear, ex.Period.FiscalQuarter}}
			for _, name := range []string{"利润表", "资产负债表", "现金流量表"} {
				data = append(data, &sheets.ValueRange{
					Range:  fmt.Sprintf("%s!A%d:B%d", name, row, row),
					Values: keys,
				})
			}
		}
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
			data = append(data, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", ref.Sheet, ref.Col, row),
				Values: [][]any{{v}},
			})
			cells++
		}
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
	return cells, nil
}

// Values reads one A1 range. It is used to confirm a batch write landed.
func (g *Google) Values(ctx context.Context, spreadsheetID, a1 string) ([][]any, error) {
	resp, err := g.svc.Spreadsheets.Values.Get(spreadsheetID, a1).
		ValueRenderOption("UNFORMATTED_VALUE").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return resp.Values, nil
}

// AsInt reads one cell from a Sheets row.
func AsInt(row []any, i int) (int, bool) {
	if i >= len(row) {
		return 0, false
	}
	return asInt(row[i])
}

func placeRow(resp *sheets.ValueRange, period model.Period) (int, bool) {
	if resp.Values == nil {
		resp.Values = [][]any{}
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
		quarter, qok := 0, false
		if len(row) > 1 {
			quarter, qok = asInt(row[1])
		}
		if yok && qok && year == period.FiscalYear && quarter == period.FiscalQuarter {
			return sheetRow, false
		}
	}
	if empty == 0 && len(resp.Values) < lastDataRow-1 {
		empty = firstDataRow + len(resp.Values)
	}
	if empty == 0 {
		return 0, false
	}
	idx := empty - firstDataRow
	for len(resp.Values) <= idx {
		resp.Values = append(resp.Values, []any{})
	}
	resp.Values[idx] = []any{float64(period.FiscalYear), float64(period.FiscalQuarter)}
	return empty, true
}

func (g *Google) ApplyCharts(ctx context.Context, spreadsheetID string, specs []model.ChartSpec) (int, error) {
	return g.ApplyCompanyCharts(ctx, spreadsheetID, specs, nil, false)
}

func (g *Google) ApplyCompanyCharts(ctx context.Context, spreadsheetID string, specs []model.ChartSpec, segments []config.Segment, keepFormulas bool) (int, error) {
	if spreadsheetID == "" {
		return 0, fmt.Errorf("这家公司还没有 spreadsheet_id。复制 Apple 那份表格模板，把新 id 写进 configs/companies.yaml")
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
	// A company that lists its own yellow cells already has the formulas and charts
	// the model reads. Painting the Apple template over them would point those charts
	// at the wrong columns.
	if keepFormulas {
		return 0, nil
	}
	if err := g.writeAnalysis(ctx, spreadsheetID, segments); err != nil {
		return 0, err
	}
	titles := map[string]tmplChart{}
	for _, spec := range specs {
		if spec.Source != "" && spec.Source != model.SourceRule {
			continue
		}
		tmpl, ok := templateByTitle(spec.Title)
		if !ok {
			continue
		}
		if len(segments) > 0 && (tmpl.Title == "硬件还是服务：本季收入从哪来" || tmpl.Title == "服务占收入的比例") {
			continue
		}
		titles[spec.Title] = tmpl
	}
	for _, tmpl := range analysisCharts {
		if len(segments) > 0 && (tmpl.Title == "硬件还是服务：本季收入从哪来" || tmpl.Title == "服务占收入的比例") {
			continue
		}
		titles[tmpl.Title] = tmpl
	}
	if len(segments) > 0 {
		pie := segmentPie(segments)
		titles[pie.Title] = pie
		trend := segmentTrend(segments)
		titles[trend.Title] = trend
	}
	var reqs []*sheets.Request
	for _, sh := range ss.Sheets {
		cols := 0
		if sh.Properties.GridProperties != nil {
			cols = int(sh.Properties.GridProperties.ColumnCount)
		}
		if cols < 40 && sh.Properties.SheetId >= 0 {
			reqs = append(reqs, &sheets.Request{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId:        sh.Properties.SheetId,
					GridProperties: &sheets.GridProperties{ColumnCount: 40},
				},
				Fields: "gridProperties.columnCount",
			}})
		}
	}
	for title, tmpl := range titles {
		if existing[title] {
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

func (g *Google) writeAnalysis(ctx context.Context, spreadsheetID string, segments []config.Segment) error {
	_, err := g.svc.Spreadsheets.Values.BatchUpdate(spreadsheetID, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
		Data:             analysisFormulasFor(segments),
	}).Context(ctx).Do()
	return err
}

func addChart(sheetID int64, tmpl tmplChart) *sheets.AddChartRequest {
	domain := grid(sheetID, tmpl.HeaderRow, tmpl.EndRow, tmpl.DomainCol, tmpl.DomainCol+1)
	cols := tmpl.SeriesCols
	if len(cols) == 0 {
		cols = []int64{tmpl.SeriesCol}
	}
	spec := &sheets.ChartSpec{Title: tmpl.Title}
	if tmpl.Type == model.ChartPie {
		series := grid(sheetID, tmpl.HeaderRow, tmpl.EndRow, cols[0], cols[0]+1)
		spec.PieChart = &sheets.PieChartSpec{
			LegendPosition: "RIGHT_LEGEND",
			Domain:         &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{domain}}},
			Series:         &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{series}}},
		}
	} else {
		legend := "NO_LEGEND"
		if len(cols) > 1 {
			legend = "BOTTOM_LEGEND"
		}
		var series []*sheets.BasicChartSeries
		for _, col := range cols {
			series = append(series, &sheets.BasicChartSeries{
				Series:     &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{grid(sheetID, tmpl.HeaderRow, tmpl.EndRow, col, col+1)}}},
				TargetAxis: "LEFT_AXIS",
			})
		}
		spec.BasicChart = &sheets.BasicChartSpec{
			ChartType:      "COLUMN",
			HeaderCount:    1,
			LegendPosition: legend,
			Domains: []*sheets.BasicChartDomain{{
				Domain: &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{domain}}},
			}},
			Series: series,
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
