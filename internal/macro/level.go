package macro

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Read separates a monthly rate file from a year-by-indicator level file.
// A level file is matched to the panel whose window and column headers fit it.
func Read(text string, panels []model.MacroPanel) (model.MacroRelease, []string, error) {
	rows := readTable(text)
	if len(rows) < 2 {
		return model.MacroRelease{}, nil, errShape()
	}
	if looksLikeYears(rows) {
		panel, ok := matchLevel(rows, panels)
		if !ok {
			window := windowIn(rows)
			if window == "" {
				return model.MacroRelease{}, nil, fmt.Errorf("这份表是历年水平值，但序列里没有 layout: level 的表。在 configs/macro.yaml 的 panels 里加一块")
			}
			return model.MacroRelease{}, nil, fmt.Errorf("没有配置历年%s。在 configs/macro.yaml 的 panels 里加一块 layout: level、window: %s", WindowLabel(window), window)
		}
		points, warnings, err := parseLevel(rows, panel)
		if err != nil {
			return model.MacroRelease{}, warnings, err
		}
		return model.MacroRelease{Levels: points, Panels: panels}, warnings, nil
	}
	points, warnings, err := parseRate(rows)
	if err != nil {
		return model.MacroRelease{}, warnings, err
	}
	return model.MacroRelease{Points: points, Panels: panels}, warnings, nil
}

func parseRate(rows [][]string) ([]model.MacroPoint, []string, error) {
	if longHeader(rows[0]) {
		return parseLong(rows)
	}
	if hasPeriod(rows) {
		return parseWide(rows)
	}
	return nil, nil, errShape()
}

func matchLevel(rows [][]string, panels []model.MacroPanel) (model.MacroPanel, bool) {
	var levels []model.MacroPanel
	for _, p := range panels {
		if p.Layout == "level" {
			levels = append(levels, p)
		}
	}
	if len(levels) == 0 || !looksLikeYears(rows) {
		return model.MacroPanel{}, false
	}
	window := windowIn(rows)
	if window != "" {
		for _, p := range levels {
			if p.Window == window {
				return p, true
			}
		}
		return model.MacroPanel{}, false
	}
	best := model.MacroPanel{}
	score := 0
	for _, p := range levels {
		n := headerScore(rows, p)
		if n > score {
			score = n
			best = p
		}
	}
	if score == 0 {
		if len(levels) == 1 {
			return levels[0], true
		}
		return model.MacroPanel{}, false
	}
	return best, true
}

func looksLikeYears(rows [][]string) bool {
	n := 0
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if _, ok := NormalizeYear(row[0]); ok {
			n++
		}
	}
	return n > 0
}

func windowIn(rows [][]string) string {
	for _, row := range rows {
		for _, cell := range row {
			if w, ok := NormalizeWindow(cell); ok {
				return w
			}
		}
	}
	return ""
}

func headerScore(rows [][]string, panel model.MacroPanel) int {
	best := 0
	for _, row := range rows {
		n := 0
		for _, cell := range row {
			if _, ok := columnByName(panel, cell); ok {
				n++
			}
		}
		if n > best {
			best = n
		}
	}
	return best
}

// NormalizeWindow accepts 1-10, 历年1-10月, and 1–10.
func NormalizeWindow(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "历年")
	s = strings.TrimSuffix(s, "月")
	s = strings.ReplaceAll(s, "—", "-")
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "~", "-")
	s = strings.ReplaceAll(s, " ", "")
	left, right, ok := strings.Cut(s, "-")
	if !ok {
		return "", false
	}
	a, errA := strconv.Atoi(left)
	b, errB := strconv.Atoi(right)
	if errA != nil || errB != nil || a < 1 || a > 12 || b < 1 || b > 12 || a > b {
		return "", false
	}
	return strconv.Itoa(a) + "-" + strconv.Itoa(b), true
}

// WindowLabel is the phrase used in titles, such as 1-10月.
func WindowLabel(window string) string {
	if window == "" {
		return ""
	}
	return window + "月"
}

// NormalizeYear accepts 2000 and 2000年.
func NormalizeYear(raw string) (int, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "年")
	if len(s) != 4 {
		return 0, false
	}
	y, err := strconv.Atoi(s)
	if err != nil || y < 1990 || y > 2100 {
		return 0, false
	}
	return y, true
}

func parseLevel(rows [][]string, panel model.MacroPanel) ([]model.LevelPoint, []string, error) {
	if longLevelHeader(rows[0]) {
		return parseLevelLong(rows, panel)
	}
	return parseLevelWide(rows, panel)
}

func longLevelHeader(header []string) bool {
	c := indexLevelHeader(header)
	return c.year >= 0 && c.indicator >= 0 && c.value >= 0
}

type levelCols struct {
	year      int
	window    int
	indicator int
	value     int
}

func indexLevelHeader(header []string) levelCols {
	c := levelCols{-1, -1, -1, -1}
	for i, h := range header {
		switch fold(h) {
		case "year", "年份", "年":
			c.year = i
		case "window", "区间", "口径":
			c.window = i
		case "indicator", "指标", "项目":
			c.indicator = i
		case "value", "数值", "值":
			c.value = i
		}
	}
	return c
}

func parseLevelLong(rows [][]string, panel model.MacroPanel) ([]model.LevelPoint, []string, error) {
	idx := indexLevelHeader(rows[0])
	var points []model.LevelPoint
	var warnings []string
	for _, row := range rows[1:] {
		year, ok := NormalizeYear(cell(row, idx.year))
		if !ok {
			continue
		}
		window := panel.Window
		if idx.window >= 0 {
			if w, ok := NormalizeWindow(cell(row, idx.window)); ok {
				window = w
			}
		}
		if window != panel.Window {
			continue
		}
		raw := cell(row, idx.indicator)
		col, ok := columnByName(panel, raw)
		if !ok {
			warnings = append(warnings, "不认识的水平指标 "+raw)
			continue
		}
		n, ok := parseNumber(cell(row, idx.value))
		if !ok {
			continue
		}
		points = append(points, model.LevelPoint{Window: panel.Window, Year: year, Indicator: col.Key, Value: &n})
	}
	if len(points) == 0 {
		return nil, warnings, fmt.Errorf("没有读到 %s 的水平值", WindowLabel(panel.Window))
	}
	return points, warnings, nil
}

func parseLevelWide(rows [][]string, panel model.MacroPanel) ([]model.LevelPoint, []string, error) {
	var header []string
	var cols []int
	var keys []string
	var points []model.LevelPoint
	var warnings []string
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if isDerivedLabel(row[0]) {
			continue
		}
		if _, ok := NormalizeYear(row[0]); ok && header != nil {
			for i, key := range keys {
				n, ok := parseNumber(cell(row, cols[i]))
				if !ok {
					continue
				}
				year, _ := NormalizeYear(row[0])
				points = append(points, model.LevelPoint{Window: panel.Window, Year: year, Indicator: key, Value: &n})
			}
			continue
		}
		foundCols := []int{}
		foundKeys := []string{}
		for i, c := range row {
			col, ok := columnByName(panel, c)
			if !ok {
				continue
			}
			foundCols = append(foundCols, i)
			foundKeys = append(foundKeys, col.Key)
		}
		if len(foundKeys) > 0 {
			header = row
			cols = foundCols
			keys = foundKeys
		}
	}
	if header == nil || len(points) == 0 {
		return nil, warnings, fmt.Errorf("没有读到 %s 的水平值。宽表第一列是年份，表头要能对上指标名", WindowLabel(panel.Window))
	}
	return points, warnings, nil
}

func columnByName(panel model.MacroPanel, raw string) (model.MacroColumn, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || isDerivedLabel(raw) {
		return model.MacroColumn{}, false
	}
	bare := stripUnit(raw)
	for _, col := range panel.Indicators {
		if col.Key == raw || col.Header == raw || stripUnit(col.Header) == bare || col.Key == fold(raw) {
			return col, true
		}
	}
	return model.MacroColumn{}, false
}

func stripUnit(s string) string {
	if i := strings.IndexAny(s, "（("); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func isDerivedLabel(s string) bool {
	switch strings.TrimSpace(s) {
	case "同比变动 (%)", "同比变动", "同比增量", "较历年同期最高 (%)", "较历年同期最高":
		return true
	default:
		return false
	}
}

// LevelSheet is one year-by-indicator grid, including formula rows and chart source.
type LevelSheet struct {
	Values [][]any
	Inputs []Cell
	Marks  []Mark
	Charts []PlacedChart
	Hide   [2]int // source column range, end exclusive; start < 0 means nothing to hide
}

// PlacedChart is a column chart whose source already sits on the grid.
type PlacedChart struct {
	Title      string
	HeaderRow  int
	EndRow     int
	DomainCol  int
	SeriesCols []int
	Colors     []string
	AnchorRow  int
	AnchorCol  int
}

// RenderLevel merges prior years with new points and puts comparison rows on formulas.
func RenderLevel(panel model.MacroPanel, existing [][]any, points []model.LevelPoint) LevelSheet {
	stored := readLevels(existing, panel)
	for _, pt := range points {
		if pt.Value == nil || pt.Window != panel.Window {
			continue
		}
		if _, ok := columnByName(panel, pt.Indicator); !ok {
			continue
		}
		stored[levelKey(pt.Year, pt.Indicator)] = *pt.Value
	}
	years := map[int]struct{}{}
	for k := range stored {
		years[yearOfKey(k)] = struct{}{}
	}
	ordered := make([]int, 0, len(years))
	for y := range years {
		ordered = append(ordered, y)
	}
	sort.Ints(ordered)

	fresh := map[string]struct{}{}
	for _, pt := range points {
		if pt.Value == nil || pt.Window != panel.Window {
			continue
		}
		fresh[levelKey(pt.Year, pt.Indicator)] = struct{}{}
	}

	n := len(panel.Indicators)
	width := 1 + n
	var sheet LevelSheet
	sheet.Hide = [2]int{-1, -1}
	title := panel.Title
	if title == "" {
		title = "历年" + WindowLabel(panel.Window)
	}
	header := make([]any, width)
	header[0] = title
	for i, col := range panel.Indicators {
		header[i+1] = col.Header
	}
	sheet.Marks = append(sheet.Marks, Mark{Kind: "header"})
	sheet.Values = append(sheet.Values, header)

	yearRow := map[int]int{}
	for _, year := range ordered {
		rowIdx := len(sheet.Values)
		yearRow[year] = rowIdx
		row := make([]any, width)
		row[0] = strconv.Itoa(year) + "年"
		for i, col := range panel.Indicators {
			k := levelKey(year, col.Key)
			v, ok := stored[k]
			if !ok {
				row[i+1] = ""
				continue
			}
			row[i+1] = v
			if _, isNew := fresh[k]; isNew {
				sheet.Inputs = append(sheet.Inputs, Cell{Row: rowIdx, Col: i + 1, Value: v, Format: "level"})
			}
		}
		sheet.Marks = append(sheet.Marks, Mark{Row: rowIdx, Kind: "level"})
		sheet.Values = append(sheet.Values, row)
	}
	if len(ordered) > 0 {
		last := len(sheet.Values)
		prev := last - 1
		first := 2
		for _, kind := range panel.Derived {
			if (kind == "yoy_pct" || kind == "yoy_delta") && len(ordered) < 2 {
				continue
			}
			rowIdx := len(sheet.Values)
			row := make([]any, width)
			row[0] = derivedName(kind)
			for i := range panel.Indicators {
				row[i+1] = derivedFormula(kind, i+1, first, prev, last)
			}
			sheet.Marks = append(sheet.Marks, Mark{Row: rowIdx, Kind: kind})
			sheet.Values = append(sheet.Values, row)
		}
	}
	sheet.Charts, sheet.Hide = placeCharts(panel, &sheet, ordered, yearRow)
	pad(&sheet)
	return sheet
}

func placeCharts(panel model.MacroPanel, sheet *LevelSheet, years []int, yearRow map[int]int) ([]PlacedChart, [2]int) {
	if len(years) == 0 || len(panel.Charts) == 0 {
		return nil, [2]int{-1, -1}
	}
	cursor := len(panel.Indicators) + 3
	hideStart := cursor
	var charts []PlacedChart
	for i, spec := range panel.Charts {
		start := spec.StartYear
		if start == 0 {
			start = years[0]
		}
		var used []int
		for _, year := range years {
			if year >= start {
				used = append(used, year)
			}
		}
		if len(used) == 0 {
			continue
		}
		domain := cursor
		seriesCols := make([]int, len(spec.Indicators))
		need := 1 + len(spec.Indicators)
		for len(sheet.Values[0]) < cursor+need {
			for r := range sheet.Values {
				sheet.Values[r] = append(sheet.Values[r], "")
			}
		}
		sheet.Values[0][domain] = "年份"
		for s, key := range spec.Indicators {
			col := columnIndex(panel, key)
			seriesCols[s] = domain + 1 + s
			sheet.Values[0][seriesCols[s]] = headerOf(panel, key)
			_ = col
		}
		for r, year := range used {
			row := r + 1
			for len(sheet.Values) <= row {
				sheet.Values = append(sheet.Values, make([]any, len(sheet.Values[0])))
				sheet.Marks = append(sheet.Marks, Mark{Row: len(sheet.Values) - 1, Kind: "source"})
			}
			src := yearRow[year]
			sheet.Values[row][domain] = fmt.Sprintf("=%s", A1(0, src))
			for s, key := range spec.Indicators {
				mainCol := columnIndex(panel, key)
				sheet.Values[row][seriesCols[s]] = fmt.Sprintf("=%s", A1(mainCol, src))
			}
		}
		title := spec.Title
		if title == "" {
			title = "历年" + WindowLabel(panel.Window)
		}
		if spec.Callout != "" {
			if spoken, ok := calloutTitle(panel, spec, years, sheet.Values, yearRow); ok {
				title = spoken
			}
		}
		charts = append(charts, PlacedChart{
			Title:      title,
			HeaderRow:  0,
			EndRow:     len(used) + 1,
			DomainCol:  domain,
			SeriesCols: seriesCols,
			Colors:     spec.Colors,
			AnchorRow:  1 + i*18,
			AnchorCol:  len(panel.Indicators) + 2,
		})
		cursor += need + 1
	}
	if hideStart >= cursor {
		return charts, [2]int{-1, -1}
	}
	return charts, [2]int{hideStart, cursor}
}

func calloutTitle(panel model.MacroPanel, spec model.MacroChart, years []int, values [][]any, yearRow map[int]int) (string, bool) {
	col := columnIndex(panel, spec.Callout)
	if col < 0 || len(years) == 0 {
		return "", false
	}
	latest := years[len(years)-1]
	lastRow := yearRow[latest]
	last, ok := asFloat(values[lastRow][col])
	if !ok {
		return "", false
	}
	peak := last
	for _, year := range years {
		n, ok := asFloat(values[yearRow[year]][col])
		if ok && n > peak {
			peak = n
		}
	}
	delta := last - peak
	name := spec.CalloutName
	if name == "" {
		name = stripUnit(headerOf(panel, spec.Callout))
	}
	unit := spokenUnit(unitOf(panel, spec.Callout))
	verb := "减少"
	if delta > 0 {
		verb = "增加"
	}
	if math.Abs(delta) < 0.05 {
		return fmt.Sprintf("历年%s：%s与历年同期最高点持平", WindowLabel(panel.Window), name), true
	}
	return fmt.Sprintf("历年%s：%s较历年同期最高点%s近%.0f%s", WindowLabel(panel.Window), name, verb, math.Round(math.Abs(delta)), unit), true
}

func spokenUnit(unit string) string {
	return strings.TrimSuffix(unit, "元")
}

func derivedFormula(kind string, col, first, prev, last int) string {
	c := colName(col)
	lastRef := c + strconv.Itoa(last)
	prevRef := c + strconv.Itoa(prev)
	span := fmt.Sprintf("%s$%d:%s", c, first, lastRef)
	switch kind {
	case "yoy_pct":
		return fmt.Sprintf(`=IF(OR(%s="",%s=""),"",ROUND((%s-%s)/%s*100,2))`, prevRef, lastRef, lastRef, prevRef, prevRef)
	case "yoy_delta":
		return fmt.Sprintf(`=IF(OR(%s="",%s=""),"",ROUND(%s-%s,2))`, prevRef, lastRef, lastRef, prevRef)
	case "vs_peak_pct":
		return fmt.Sprintf(`=IF(%s="","",ROUND((%s-MAX(%s))/MAX(%s)*100,2))`, lastRef, lastRef, span, span)
	case "vs_peak_delta":
		return fmt.Sprintf(`=IF(%s="","",ROUND(%s-MAX(%s),2))`, lastRef, lastRef, span)
	default:
		return ""
	}
}

func derivedName(kind string) string {
	switch kind {
	case "yoy_pct":
		return "同比变动 (%)"
	case "yoy_delta":
		return "同比增量"
	case "vs_peak_pct":
		return "较历年同期最高 (%)"
	case "vs_peak_delta":
		return "较历年同期最高"
	default:
		return kind
	}
}

func readLevels(existing [][]any, panel model.MacroPanel) map[string]float64 {
	stored := map[string]float64{}
	if len(existing) < 2 {
		return stored
	}
	cols := map[int]string{}
	if len(existing) > 0 {
		for i, cell := range existing[0] {
			text, ok := asText(cell)
			if !ok {
				continue
			}
			col, ok := columnByName(panel, text)
			if ok {
				cols[i] = col.Key
			}
		}
	}
	for _, row := range existing[1:] {
		if len(row) == 0 {
			continue
		}
		label, _ := asText(row[0])
		if isDerivedLabel(label) {
			break
		}
		year, ok := NormalizeYear(label)
		if !ok {
			continue
		}
		for i, key := range cols {
			if i >= len(row) {
				continue
			}
			n, ok := asFloat(row[i])
			if !ok {
				continue
			}
			stored[levelKey(year, key)] = n
		}
	}
	return stored
}

func pad(sheet *LevelSheet) {
	width := 0
	for _, row := range sheet.Values {
		if len(row) > width {
			width = len(row)
		}
	}
	for i := range sheet.Values {
		for len(sheet.Values[i]) < width {
			sheet.Values[i] = append(sheet.Values[i], "")
		}
	}
}

func levelKey(year int, indicator string) string {
	return strconv.Itoa(year) + "\t" + indicator
}

func yearOfKey(key string) int {
	y, _ := strconv.Atoi(strings.SplitN(key, "\t", 2)[0])
	return y
}

func columnIndex(panel model.MacroPanel, key string) int {
	for i, col := range panel.Indicators {
		if col.Key == key {
			return i + 1
		}
	}
	return -1
}

func headerOf(panel model.MacroPanel, key string) string {
	for _, col := range panel.Indicators {
		if col.Key == key {
			return col.Header
		}
	}
	return key
}

func unitOf(panel model.MacroPanel, key string) string {
	for _, col := range panel.Indicators {
		if col.Key == key {
			return col.Unit
		}
	}
	return ""
}

// Brief describes the latest year of a level panel: level, year-on-year, and distance from the peak.
func Brief(rel model.MacroRelease) []string {
	if len(rel.Levels) > 0 {
		return levelBrief(rel)
	}
	return Summary(rel.Points)
}

func levelBrief(rel model.MacroRelease) []string {
	var lines []string
	for _, panel := range rel.Panels {
		if panel.Layout != "level" {
			continue
		}
		var mine []model.LevelPoint
		for _, pt := range rel.Levels {
			if pt.Window == panel.Window && pt.Value != nil {
				mine = append(mine, pt)
			}
		}
		if len(mine) == 0 {
			continue
		}
		sheet := RenderLevel(panel, nil, mine)
		latest := 0
		for _, pt := range mine {
			if pt.Year > latest {
				latest = pt.Year
			}
		}
		for _, col := range panel.Indicators {
			n, ok := levelValue(sheet, latest, col.Key, panel)
			if !ok {
				continue
			}
			parts := fmt.Sprintf("%d年%s %s %s%s", latest, WindowLabel(panel.Window), stripUnit(col.Header), trimFloat(n), col.Unit)
			if yoy, ok := evalYoy(sheet, panel, col.Key); ok {
				parts += "，同比 " + strconv.FormatFloat(yoy, 'f', 2, 64) + "%"
			}
			if gap, ok := evalPeak(sheet, panel, col.Key); ok {
				parts += "，较历年同期最高 " + strconv.FormatFloat(gap, 'f', 2, 64) + "%"
			}
			lines = append(lines, parts)
		}
	}
	return lines
}

func levelValue(sheet LevelSheet, year int, key string, panel model.MacroPanel) (float64, bool) {
	col := columnIndex(panel, key)
	for i, row := range sheet.Values {
		if i == 0 || len(row) == 0 {
			continue
		}
		label, _ := asText(row[0])
		y, ok := NormalizeYear(label)
		if ok && y == year && col < len(row) {
			return asFloat(row[col])
		}
	}
	return 0, false
}

func evalYoy(sheet LevelSheet, panel model.MacroPanel, key string) (float64, bool) {
	col := columnIndex(panel, key)
	var prev, last *float64
	for i, row := range sheet.Values {
		if i == 0 || col >= len(row) {
			continue
		}
		label, _ := asText(row[0])
		if _, ok := NormalizeYear(label); !ok {
			continue
		}
		n, ok := asFloat(row[col])
		if !ok {
			continue
		}
		prev = last
		v := n
		last = &v
	}
	if prev == nil || last == nil || *prev == 0 {
		return 0, false
	}
	return math.Round(((*last-*prev) / *prev * 100)*100) / 100, true
}

func evalPeak(sheet LevelSheet, panel model.MacroPanel, key string) (float64, bool) {
	col := columnIndex(panel, key)
	var last float64
	var peak float64
	seen := false
	for i, row := range sheet.Values {
		if i == 0 || col >= len(row) {
			continue
		}
		label, _ := asText(row[0])
		if _, ok := NormalizeYear(label); !ok {
			continue
		}
		n, ok := asFloat(row[col])
		if !ok {
			continue
		}
		if !seen || n > peak {
			peak = n
		}
		last = n
		seen = true
	}
	if !seen || peak == 0 {
		return 0, false
	}
	return math.Round(((last-peak)/peak*100)*100) / 100, true
}

func trimFloat(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// ChartSpecs is the chart list a level file will create. Titles already include the latest peak gap.
func ChartSpecs(rel model.MacroRelease) []model.ChartSpec {
	var out []model.ChartSpec
	for _, panel := range rel.Panels {
		if panel.Layout != "level" {
			continue
		}
		var mine []model.LevelPoint
		for _, pt := range rel.Levels {
			if pt.Window == panel.Window {
				mine = append(mine, pt)
			}
		}
		if len(mine) == 0 {
			continue
		}
		sheet := RenderLevel(panel, nil, mine)
		for _, ch := range sheet.Charts {
			out = append(out, model.ChartSpec{
				Title:     ch.Title,
				ChartType: model.ChartColumn,
				Sheet:     panel.Sheet,
				Source:    model.SourceRule,
				Reason:    "历年同期水平",
			})
		}
	}
	return out
}
