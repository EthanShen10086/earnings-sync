package macro

import (
	"encoding/csv"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Parse reads either a long CSV (period, basis, indicator, value) or the wide
// sheet itself (a section header, then one indicator per row, months across).
func Parse(text string) ([]model.MacroPoint, []string, error) {
	rows := readTable(text)
	if len(rows) < 2 {
		return nil, nil, errShape()
	}
	if longHeader(rows[0]) {
		return parseLong(rows)
	}
	if hasPeriod(rows) {
		return parseWide(rows)
	}
	return nil, nil, errShape()
}

func errShape() error {
	return errString("这份文件不是宏观表。长表需要 period、basis、indicator、value；宽表第一列是指标，后面各列是 202610 这样的月份")
}

type errString string

func (e errString) Error() string { return string(e) }

func readTable(text string) [][]string {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	raw, err := r.ReadAll()
	if err != nil {
		return splitLoose(text)
	}
	var rows [][]string
	for _, row := range raw {
		if len(row) == 0 || strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			continue
		}
		blank := true
		for i := range row {
			row[i] = strings.TrimSpace(row[i])
			if row[i] != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func splitLoose(text string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rows = append(rows, strings.Split(line, ","))
	}
	return rows
}

func longHeader(header []string) bool {
	idx := indexHeader(header)
	return idx.period >= 0 && idx.indicator >= 0 && idx.value >= 0
}

type cols struct {
	period    int
	basis     int
	indicator int
	value     int
}

func indexHeader(header []string) cols {
	c := cols{-1, -1, -1, -1}
	for i, h := range header {
		switch fold(h) {
		case "period", "month", "期间", "月份", "时间":
			c.period = i
		case "basis", "measure", "口径":
			c.basis = i
		case "indicator", "指标", "项目", "lineitem":
			c.indicator = i
		case "value", "数值", "值", "同比":
			c.value = i
		}
	}
	return c
}

func parseLong(rows [][]string) ([]model.MacroPoint, []string, error) {
	idx := indexHeader(rows[0])
	var points []model.MacroPoint
	var warnings []string
	seen := map[string]int{}
	for _, row := range rows[1:] {
		period, ok := NormalizePeriod(cell(row, idx.period))
		if !ok {
			continue
		}
		raw := cell(row, idx.indicator)
		basis := ""
		if idx.basis >= 0 {
			basis = NormalizeBasis(cell(row, idx.basis))
		}
		if basis == "" {
			basis = inferBasis(raw)
		}
		if basis == "" {
			warnings = append(warnings, "缺少口径，已跳过 "+raw)
			continue
		}
		n, ok := parseNumber(cell(row, idx.value))
		if !ok {
			continue
		}
		token := basis + "\t" + period + "\t" + raw
		ind, ok := Resolve(basis, raw, seen[token])
		seen[token]++
		if !ok {
			warnings = append(warnings, "不认识的指标 "+raw)
			continue
		}
		points = append(points, model.MacroPoint{Period: period, Basis: ind.Basis, Indicator: ind.Key, Value: &n})
	}
	if len(points) == 0 {
		return nil, warnings, errShape()
	}
	return points, warnings, nil
}

type wideHeader struct {
	basisCol     int
	indicatorCol int
	section      string
	periods      []periodCol
}

type periodCol struct {
	index  int
	period string
}

func parseWide(rows [][]string) ([]model.MacroPoint, []string, error) {
	var header *wideHeader
	var points []model.MacroPoint
	var warnings []string
	occ := map[string]int{}
	for _, row := range rows {
		if h, ok := parseWideHeader(row); ok {
			header = &h
			occ = map[string]int{}
			continue
		}
		if header == nil {
			continue
		}
		basis := header.section
		if header.basisCol >= 0 {
			if b := NormalizeBasis(cell(row, header.basisCol)); b != "" {
				basis = b
			}
		}
		raw := cell(row, header.indicatorCol)
		if basis == "" {
			basis = inferBasis(raw)
		}
		if raw == "" || NormalizeBasis(raw) != "" {
			continue
		}
		ind, ok := Resolve(basis, raw, occ[raw])
		occ[raw]++
		if !ok {
			warnings = append(warnings, "不认识的指标 "+raw)
			continue
		}
		for _, pc := range header.periods {
			n, ok := parseNumber(cell(row, pc.index))
			if !ok {
				continue
			}
			points = append(points, model.MacroPoint{Period: pc.period, Basis: ind.Basis, Indicator: ind.Key, Value: &n})
		}
	}
	if len(points) == 0 {
		return nil, warnings, errShape()
	}
	return points, warnings, nil
}

func parseWideHeader(row []string) (wideHeader, bool) {
	var periods []periodCol
	for i, c := range row {
		if p, ok := NormalizePeriod(c); ok {
			periods = append(periods, periodCol{index: i, period: p})
		}
	}
	if len(periods) == 0 {
		return wideHeader{}, false
	}
	h := wideHeader{basisCol: -1, indicatorCol: 0, periods: periods}
	if b := NormalizeBasis(row[0]); b != "" {
		h.section = b
		return h, true
	}
	for i, c := range row {
		switch fold(c) {
		case "basis", "口径", "measure":
			h.basisCol = i
		case "indicator", "指标", "项目", "lineitem":
			h.indicatorCol = i
		}
	}
	return h, true
}

func hasPeriod(rows [][]string) bool {
	for _, row := range rows {
		for _, c := range row {
			if _, ok := NormalizePeriod(c); ok {
				return true
			}
		}
	}
	return false
}

func inferBasis(raw string) string {
	if strings.Contains(raw, "环比") {
		return BasisMoM
	}
	if strings.Contains(raw, "当季") {
		return BasisYoY
	}
	return ""
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// NormalizePeriod accepts 202610, 2026-10, and 2026年10月.
func NormalizePeriod(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "月")
	s = strings.ReplaceAll(s, "年", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, ".", "")
	if len(s) != 6 {
		return "", false
	}
	year, errY := strconv.Atoi(s[:4])
	month, errM := strconv.Atoi(s[4:])
	if errY != nil || errM != nil || year < 1990 || year > 2100 || month < 1 || month > 12 {
		return "", false
	}
	return s, true
}

func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	switch s {
	case "", "-", "—", "–", "NA", "N/A", "null":
		return 0, false
	}
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil
}
