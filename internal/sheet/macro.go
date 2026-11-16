package sheet

import (
	"context"
	"fmt"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/macro"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"google.golang.org/api/sheets/v4"
)

// CreateMacroWorkbook makes the monthly rate sheet and shares it when an address is set.
func (g *Google) CreateMacroWorkbook(ctx context.Context, credFile, title, shareWith string) (string, error) {
	if title == "" {
		title = "中国宏观经济数据汇总"
	}
	ss, err := g.svc.Spreadsheets.Create(&sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{Title: title},
		Sheets:     []*sheets.Sheet{{Properties: &sheets.SheetProperties{Title: macro.SheetName}}},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("创建宏观表格失败：%w。请确认这个 Cloud 项目已启用 Google Drive API", err)
	}
	if _, err := g.UpsertMacro(ctx, ss.SpreadsheetId, model.MacroRelease{Title: title}); err != nil {
		return ss.SpreadsheetId, fmt.Errorf("表已创建但表头写入失败：%w", err)
	}
	if shareWith != "" {
		if err := shareEdit(ctx, credFile, ss.SpreadsheetId, shareWith); err != nil {
			return ss.SpreadsheetId, err
		}
	}
	return ss.SpreadsheetId, nil
}

// UpsertMacro writes this month's rates into 宏观 and leaves older months in place.
func (g *Google) UpsertMacro(ctx context.Context, spreadsheetID string, rel model.MacroRelease) (int, error) {
	if spreadsheetID == "" {
		return 0, fmt.Errorf("这个宏观序列还没有 spreadsheet_id。先运行 go run ./cmd/sheetcreate -series CN")
	}
	sh, err := g.ensureSheet(ctx, spreadsheetID, macro.SheetName)
	if err != nil {
		return 0, err
	}
	resp, err := g.svc.Spreadsheets.Values.Get(spreadsheetID, a1(macro.SheetName, "A1:ZZ400")).
		ValueRenderOption("UNFORMATTED_VALUE").Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	grid := macro.Render(rel.Title, resp.Values, rel.Points)
	if err := g.fitMacroGrid(ctx, spreadsheetID, sh, len(grid.Values), widthOf(grid.Values)); err != nil {
		return 0, err
	}
	_, err = g.svc.Spreadsheets.Values.BatchUpdate(spreadsheetID, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data: []*sheets.ValueRange{{
			Range:  a1(macro.SheetName, "A1"),
			Values: grid.Values,
		}},
	}).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	if err := g.formatMacro(ctx, spreadsheetID, sh, grid); err != nil {
		return len(grid.Cells), err
	}
	return len(grid.Cells), nil
}

func (g *Google) ensureSheet(ctx context.Context, spreadsheetID, name string) (*sheets.Sheet, error) {
	ss, err := g.svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties,conditionalFormats,charts)").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	for _, sh := range ss.Sheets {
		if sh.Properties != nil && sh.Properties.Title == name {
			return sh, nil
		}
	}
	resp, err := g.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			AddSheet: &sheets.AddSheetRequest{Properties: &sheets.SheetProperties{Title: name}},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if len(resp.Replies) == 0 || resp.Replies[0].AddSheet == nil {
		return nil, fmt.Errorf("工作表 %s 已添加，但没有返回表 id", name)
	}
	return &sheets.Sheet{Properties: resp.Replies[0].AddSheet.Properties}, nil
}

func a1(sheetName, cells string) string {
	if strings.ContainsAny(sheetName, " !'") {
		return "'" + strings.ReplaceAll(sheetName, "'", "''") + "'!" + cells
	}
	return sheetName + "!" + cells
}

func (g *Google) fitMacroGrid(ctx context.Context, spreadsheetID string, sh *sheets.Sheet, rows, cols int) error {
	if sh.Properties == nil {
		return nil
	}
	props := sh.Properties.GridProperties
	needRows, needCols := int64(rows+5), int64(cols+2)
	if props != nil {
		if props.RowCount > needRows {
			needRows = props.RowCount
		}
		if props.ColumnCount > needCols {
			needCols = props.ColumnCount
		}
		if props.RowCount >= needRows && props.ColumnCount >= needCols && props.FrozenColumnCount >= 1 {
			return nil
		}
	}
	_, err := g.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId: sh.Properties.SheetId,
					GridProperties: &sheets.GridProperties{
						RowCount:          needRows,
						ColumnCount:       needCols,
						FrozenColumnCount: 1,
					},
				},
				Fields: "gridProperties.rowCount,gridProperties.columnCount,gridProperties.frozenColumnCount",
			},
		}},
	}).Context(ctx).Do()
	return err
}

func (g *Google) formatMacro(ctx context.Context, spreadsheetID string, sh *sheets.Sheet, grid macro.Sheet) error {
	if sh.Properties == nil || len(grid.Values) == 0 {
		return nil
	}
	sid := sh.Properties.SheetId
	cols := int64(widthOf(grid.Values))
	var reqs []*sheets.Request
	reqs = append(reqs, &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
		Range:      &sheets.DimensionRange{SheetId: sid, Dimension: "COLUMNS", StartIndex: 0, EndIndex: 1},
		Properties: &sheets.DimensionProperties{PixelSize: 240},
		Fields:     "pixelSize",
	}})
	if cols > 1 {
		reqs = append(reqs, &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
			Range:      &sheets.DimensionRange{SheetId: sid, Dimension: "COLUMNS", StartIndex: 1, EndIndex: cols},
			Properties: &sheets.DimensionProperties{PixelSize: 78},
			Fields:     "pixelSize",
		}})
	}
	var dataRanges []*sheets.GridRange
	for _, mark := range grid.Marks {
		switch mark.Kind {
		case "title":
			reqs = append(reqs, repeatText(sid, mark.Row, 0, 1, true, color(0.15, 0.15, 0.15), color(1, 1, 1)))
		case "header":
			reqs = append(reqs, repeatText(sid, mark.Row, 0, cols, true, color(0.25, 0.22, 0.1), color(0.99, 0.94, 0.78)))
		case "percent":
			reqs = append(reqs, repeatNumber(sid, mark.Row, 1, cols, `0.##"%"`))
			dataRanges = append(dataRanges, gridRange(sid, mark.Row, 1, cols))
		case "number":
			reqs = append(reqs, repeatNumber(sid, mark.Row, 1, cols, "0.0"))
			dataRanges = append(dataRanges, gridRange(sid, mark.Row, 1, cols))
		}
	}
	if len(sh.ConditionalFormats) == 0 && len(dataRanges) > 0 {
		reqs = append(reqs,
			signRule(dataRanges, "NUMBER_GREATER_THAN_EQ", color(0.11, 0.42, 0.2), color(0.86, 0.94, 0.86)),
			signRule(dataRanges, "NUMBER_LESS", color(0.7, 0.13, 0.13), color(0.98, 0.88, 0.86)),
		)
	}
	_, err := g.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: reqs}).Context(ctx).Do()
	return err
}

func repeatText(sheetID int64, row int, c1, c2 int64, bold bool, fg, bg *sheets.Color) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: grid(sheetID, int64(row), int64(row)+1, c1, c2),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			TextFormat:      &sheets.TextFormat{Bold: bold, ForegroundColor: fg},
			BackgroundColor: bg,
		}},
		Fields: "userEnteredFormat(textFormat,backgroundColor)",
	}}
}

func repeatNumber(sheetID int64, row int, c1, c2 int64, pattern string) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: grid(sheetID, int64(row), int64(row)+1, c1, c2),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			NumberFormat:        &sheets.NumberFormat{Type: "NUMBER", Pattern: pattern},
			HorizontalAlignment: "CENTER",
		}},
		Fields: "userEnteredFormat(numberFormat,horizontalAlignment)",
	}}
}

func signRule(ranges []*sheets.GridRange, cond string, fg, bg *sheets.Color) *sheets.Request {
	return &sheets.Request{AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
		Index: 0,
		Rule: &sheets.ConditionalFormatRule{
			Ranges: ranges,
			BooleanRule: &sheets.BooleanRule{
				Condition: &sheets.BooleanCondition{
					Type:   cond,
					Values: []*sheets.ConditionValue{{UserEnteredValue: "0"}},
				},
				Format: &sheets.CellFormat{
					TextFormat:      &sheets.TextFormat{ForegroundColor: fg},
					BackgroundColor: bg,
				},
			},
		},
	}}
}

func gridRange(sheetID int64, row int, c1, c2 int64) *sheets.GridRange {
	return grid(sheetID, int64(row), int64(row)+1, c1, c2)
}

func color(r, g, b float64) *sheets.Color {
	return &sheets.Color{Red: r, Green: g, Blue: b}
}

func widthOf(rows [][]any) int {
	n := 0
	for _, row := range rows {
		if len(row) > n {
			n = len(row)
		}
	}
	return n
}
