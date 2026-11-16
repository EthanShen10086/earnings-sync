package sheet

import (
	"context"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/macro"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"google.golang.org/api/sheets/v4"
)

func (g *Google) upsertLevel(ctx context.Context, spreadsheetID string, panel model.MacroPanel, points []model.LevelPoint) (int, error) {
	sh, err := g.ensureSheet(ctx, spreadsheetID, panel.Sheet)
	if err != nil {
		return 0, err
	}
	resp, err := g.svc.Spreadsheets.Values.Get(spreadsheetID, a1(panel.Sheet, "A1:ZZ400")).
		ValueRenderOption("UNFORMATTED_VALUE").Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	grid := macro.RenderLevel(panel, resp.Values, points)
	if err := g.fitMacroGrid(ctx, spreadsheetID, sh, len(grid.Values), widthOf(grid.Values), true); err != nil {
		return 0, err
	}
	_, err = g.svc.Spreadsheets.Values.BatchUpdate(spreadsheetID, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
		Data: []*sheets.ValueRange{{
			Range:  a1(panel.Sheet, "A1"),
			Values: grid.Values,
		}},
	}).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	if err := g.formatLevel(ctx, spreadsheetID, sh, grid); err != nil {
		return len(grid.Inputs), err
	}
	if err := g.syncLevelCharts(ctx, spreadsheetID, panel.Sheet, grid); err != nil {
		return len(grid.Inputs), err
	}
	return len(grid.Inputs), nil
}

func (g *Google) formatLevel(ctx context.Context, spreadsheetID string, sh *sheets.Sheet, grid macro.LevelSheet) error {
	if sh.Properties == nil || len(grid.Values) == 0 {
		return nil
	}
	sid := sh.Properties.SheetId
	dataCols := int64(1)
	if grid.Hide[0] > 1 {
		dataCols = int64(grid.Hide[0] - 2)
	} else if len(grid.Values[0]) > 1 {
		dataCols = int64(len(grid.Values[0]))
	}
	if dataCols < 2 {
		dataCols = 2
	}
	var reqs []*sheets.Request
	reqs = append(reqs, &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
		Range:      &sheets.DimensionRange{SheetId: sid, Dimension: "COLUMNS", StartIndex: 0, EndIndex: 1},
		Properties: &sheets.DimensionProperties{PixelSize: 160},
		Fields:     "pixelSize",
	}})
	reqs = append(reqs, &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
		Range:      &sheets.DimensionRange{SheetId: sid, Dimension: "COLUMNS", StartIndex: 1, EndIndex: dataCols},
		Properties: &sheets.DimensionProperties{PixelSize: 180},
		Fields:     "pixelSize",
	}})
	if grid.Hide[0] >= 0 && grid.Hide[1] > grid.Hide[0] {
		reqs = append(reqs, &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
			Range:      &sheets.DimensionRange{SheetId: sid, Dimension: "COLUMNS", StartIndex: int64(grid.Hide[0]), EndIndex: int64(grid.Hide[1])},
			Properties: &sheets.DimensionProperties{HiddenByUser: true},
			Fields:     "hiddenByUser",
		}})
	}
	var painted []*sheets.GridRange
	for _, mark := range grid.Marks {
		switch mark.Kind {
		case "header":
			reqs = append(reqs, repeatText(sid, mark.Row, 0, dataCols, true, color(0.25, 0.22, 0.1), color(0.99, 0.94, 0.78)))
		case "level":
			reqs = append(reqs, repeatNumber(sid, mark.Row, 1, dataCols, "0.00"))
		case "yoy_pct", "vs_peak_pct":
			reqs = append(reqs, repeatText(sid, mark.Row, 0, 1, true, color(0.2, 0.16, 0.05), color(0.98, 0.93, 0.7)))
			reqs = append(reqs, repeatNumber(sid, mark.Row, 1, dataCols, "0.00"))
			reqs = append(reqs, repeatText(sid, mark.Row, 1, dataCols, true, color(0.2, 0.16, 0.05), color(0.98, 0.93, 0.7)))
			painted = append(painted, gridRange(sid, mark.Row, 1, dataCols))
		case "yoy_delta", "vs_peak_delta":
			reqs = append(reqs, repeatNumber(sid, mark.Row, 1, dataCols, "0.00"))
			painted = append(painted, gridRange(sid, mark.Row, 1, dataCols))
		}
	}
	if len(sh.ConditionalFormats) == 0 && len(painted) > 0 {
		reqs = append(reqs,
			signInk(painted, "NUMBER_GREATER", color(0.11, 0.42, 0.2)),
			signInk(painted, "NUMBER_LESS", color(0.7, 0.13, 0.13)),
		)
	}
	if len(reqs) == 0 {
		return nil
	}
	_, err := g.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: reqs}).Context(ctx).Do()
	return err
}

func (g *Google) syncLevelCharts(ctx context.Context, spreadsheetID, sheetName string, grid macro.LevelSheet) error {
	if len(grid.Charts) == 0 {
		return nil
	}
	sh, err := g.ensureSheet(ctx, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	if sh.Properties == nil {
		return nil
	}
	var reqs []*sheets.Request
	for _, ch := range sh.Charts {
		reqs = append(reqs, &sheets.Request{DeleteEmbeddedObject: &sheets.DeleteEmbeddedObjectRequest{ObjectId: ch.ChartId}})
	}
	sid := sh.Properties.SheetId
	for _, ch := range grid.Charts {
		reqs = append(reqs, &sheets.Request{AddChart: levelChart(sid, ch)})
	}
	_, err = g.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: reqs}).Context(ctx).Do()
	return err
}

func levelChart(sheetID int64, ch macro.PlacedChart) *sheets.AddChartRequest {
	domain := grid(sheetID, int64(ch.HeaderRow), int64(ch.EndRow), int64(ch.DomainCol), int64(ch.DomainCol)+1)
	series := make([]*sheets.BasicChartSeries, 0, len(ch.SeriesCols))
	for i, col := range ch.SeriesCols {
		item := &sheets.BasicChartSeries{
			Series:     &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{grid(sheetID, int64(ch.HeaderRow), int64(ch.EndRow), int64(col), int64(col)+1)}}},
			TargetAxis: "LEFT_AXIS",
			DataLabel:  &sheets.DataLabel{Type: "DATA", Placement: "OUTSIDE_END", TextFormat: &sheets.TextFormat{FontSize: 9}},
		}
		if i < len(ch.Colors) {
			if c, ok := parseHex(ch.Colors[i]); ok {
				item.ColorStyle = &sheets.ColorStyle{RgbColor: c}
			}
		}
		series = append(series, item)
	}
	return &sheets.AddChartRequest{Chart: &sheets.EmbeddedChart{
		Spec: &sheets.ChartSpec{
			Title: ch.Title,
			BasicChart: &sheets.BasicChartSpec{
				ChartType:      "COLUMN",
				HeaderCount:    1,
				LegendPosition: "BOTTOM_LEGEND",
				Domains: []*sheets.BasicChartDomain{{
					Domain: &sheets.ChartData{SourceRange: &sheets.ChartSourceRange{Sources: []*sheets.GridRange{domain}}},
				}},
				Series: series,
			},
		},
		Position: &sheets.EmbeddedObjectPosition{
			OverlayPosition: &sheets.OverlayPosition{
				AnchorCell:   &sheets.GridCoordinate{SheetId: sheetID, RowIndex: int64(ch.AnchorRow), ColumnIndex: int64(ch.AnchorCol)},
				WidthPixels:  980,
				HeightPixels: 360,
			},
		},
	}}
}

func signInk(ranges []*sheets.GridRange, cond string, fg *sheets.Color) *sheets.Request {
	return &sheets.Request{AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
		Index: 0,
		Rule: &sheets.ConditionalFormatRule{
			Ranges: ranges,
			BooleanRule: &sheets.BooleanRule{
				Condition: &sheets.BooleanCondition{
					Type:   cond,
					Values: []*sheets.ConditionValue{{UserEnteredValue: "0"}},
				},
				Format: &sheets.CellFormat{TextFormat: &sheets.TextFormat{ForegroundColor: fg}},
			},
		},
	}}
}

func parseHex(s string) (*sheets.Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return nil, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil, false
	}
	return &sheets.Color{
		Red:   float64((n>>16)&0xff) / 255,
		Green: float64((n>>8)&0xff) / 255,
		Blue:  float64(n&0xff) / 255,
	}, true
}
