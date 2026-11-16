package macro

import (
	"sort"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Mark tells the sheet writer how to format one row.
type Mark struct {
	Row  int
	Kind string // title, header, percent, number
}

// Cell is one value written by this release, not a value already on the sheet.
type Cell struct {
	Row    int
	Col    int
	Value  float64
	Format string
}

// Sheet is the full macro grid, ready to replace the used range.
type Sheet struct {
	Values [][]any
	Cells  []Cell
	Marks  []Mark
}

// Render merges existing sheet values with new points. Months stay in chronological order.
func Render(title string, existing [][]any, points []model.MacroPoint) Sheet {
	stored, keptTitle := readGrid(existing)
	if title == "" {
		title = keptTitle
	}
	if title == "" {
		title = "中国宏观经济数据汇总"
	}
	for _, pt := range points {
		if pt.Value == nil {
			continue
		}
		ind, ok := Resolve(pt.Basis, pt.Indicator, 0)
		if !ok {
			continue
		}
		stored[storeKey(ind.Basis, ind.Key, pt.Period)] = *pt.Value
	}
	periods := map[string]struct{}{}
	for k := range stored {
		periods[periodOf(k)] = struct{}{}
	}
	ordered := make([]string, 0, len(periods))
	for p := range periods {
		if p != "" {
			ordered = append(ordered, p)
		}
	}
	sort.Strings(ordered)

	var sheet Sheet
	width := 1 + len(ordered)
	titleRow := make([]any, width)
	titleRow[0] = title
	sheet.Values = append(sheet.Values, titleRow)
	sheet.Marks = append(sheet.Marks, Mark{Kind: "title"})

	fresh := map[string]struct{}{}
	for _, pt := range points {
		if pt.Value == nil {
			continue
		}
		ind, ok := Resolve(pt.Basis, pt.Indicator, 0)
		if !ok {
			continue
		}
		fresh[storeKey(ind.Basis, ind.Key, pt.Period)] = struct{}{}
	}

	for _, basis := range []string{BasisYTD, BasisYoY, BasisMoM} {
		header := make([]any, width)
		header[0] = BasisLabel(basis)
		for i, p := range ordered {
			header[i+1] = p
		}
		sheet.Marks = append(sheet.Marks, Mark{Row: len(sheet.Values), Kind: "header"})
		sheet.Values = append(sheet.Values, header)
		for _, ind := range Indicators(basis) {
			rowIdx := len(sheet.Values)
			row := make([]any, width)
			row[0] = ind.Label
			for i, p := range ordered {
				k := storeKey(basis, ind.Key, p)
				n, ok := stored[k]
				if !ok {
					row[i+1] = ""
					continue
				}
				row[i+1] = n
				if _, isNew := fresh[k]; isNew {
					sheet.Cells = append(sheet.Cells, Cell{Row: rowIdx, Col: i + 1, Value: n, Format: ind.Format})
				}
			}
			sheet.Marks = append(sheet.Marks, Mark{Row: rowIdx, Kind: ind.Format})
			sheet.Values = append(sheet.Values, row)
		}
	}
	return sheet
}

func readGrid(existing [][]any) (map[string]float64, string) {
	stored := map[string]float64{}
	if len(existing) == 0 {
		return stored, ""
	}
	title := ""
	if len(existing[0]) > 0 {
		if s, ok := asText(existing[0][0]); ok && NormalizeBasis(s) == "" {
			title = s
		}
	}
	basis := ""
	var periods []string
	occ := map[string]int{}
	for _, row := range existing {
		if len(row) == 0 {
			continue
		}
		label, _ := asText(row[0])
		if b := NormalizeBasis(label); b != "" {
			basis = b
			periods = periodsFrom(row)
			occ = map[string]int{}
			continue
		}
		if basis == "" || label == "" {
			continue
		}
		ind, ok := Resolve(basis, label, occ[label])
		if !ok {
			continue
		}
		occ[label]++
		for i, p := range periods {
			if p == "" || i+1 >= len(row) {
				continue
			}
			n, ok := asFloat(row[i+1])
			if !ok {
				continue
			}
			stored[storeKey(ind.Basis, ind.Key, p)] = n
		}
	}
	return stored, title
}

func periodsFrom(row []any) []string {
	out := make([]string, 0, len(row)-1)
	for _, cell := range row[1:] {
		if p, ok := asPeriod(cell); ok {
			out = append(out, p)
			continue
		}
		out = append(out, "")
	}
	return out
}

func storeKey(basis, key, period string) string {
	return basis + "\t" + key + "\t" + period
}

func periodOf(key string) string {
	i := len(key) - 1
	for i >= 0 && key[i] != '\t' {
		i--
	}
	if i < 0 {
		return ""
	}
	return key[i+1:]
}

func asText(v any) (string, bool) {
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n), true
	case float64:
		if p, ok := asPeriod(n); ok {
			return p, true
		}
		return strconv.FormatFloat(n, 'f', -1, 64), true
	case int:
		return strconv.Itoa(n), true
	default:
		return "", false
	}
}

func asPeriod(v any) (string, bool) {
	switch n := v.(type) {
	case string:
		return NormalizePeriod(n)
	case float64:
		if n < 199001 || n > 210012 || n != float64(int(n)) {
			return "", false
		}
		return NormalizePeriod(strconv.Itoa(int(n)))
	case int:
		return NormalizePeriod(strconv.Itoa(n))
	default:
		return "", false
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if _, isPeriod := asPeriod(n); isPeriod && n >= 199001 {
			return 0, false
		}
		return n, true
	case int:
		return float64(n), true
	case string:
		return parseNumber(n)
	default:
		return 0, false
	}
}

// A1 converts a zero-based cell into spreadsheet notation.
func A1(col, row int) string {
	return colName(col) + strconv.Itoa(row+1)
}

func colName(col int) string {
	col++
	var b [8]byte
	i := len(b)
	for col > 0 {
		col--
		i--
		b[i] = byte('A' + col%26)
		col /= 26
	}
	return string(b[i:])
}
