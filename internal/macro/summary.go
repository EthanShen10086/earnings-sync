package macro

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// LatestPeriod is the newest YYYYMM among the points.
func LatestPeriod(points []model.MacroPoint) string {
	latest := ""
	for _, pt := range points {
		if pt.Period > latest {
			latest = pt.Period
		}
	}
	return latest
}

// Summary is a short factual note for the newest month. It does not invent a story.
func Summary(points []model.MacroPoint) []string {
	latest := LatestPeriod(points)
	if latest == "" {
		return nil
	}
	var lines []string
	for _, basis := range []string{BasisYTD, BasisYoY, BasisMoM} {
		line := lineFor(basis, latest, points)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func lineFor(basis, period string, points []model.MacroPoint) string {
	got := map[string]float64{}
	for _, pt := range points {
		if pt.Value == nil || pt.Period != period || NormalizeBasis(pt.Basis) != basis {
			continue
		}
		ind, ok := Resolve(pt.Basis, pt.Indicator, 0)
		if !ok {
			continue
		}
		got[ind.Key] = *pt.Value
	}
	if len(got) == 0 {
		return ""
	}
	order := []string{"gdp", "retail", "fai", "industrial", "industrial_profit", "trade", "exports", "imports", "cpi", "ppi", "cpi_ppi", "pmi", "property_investment", "property_sales"}
	var parts []string
	for _, key := range order {
		n, ok := got[key]
		if !ok {
			continue
		}
		ind, ok := Resolve(basis, key, 0)
		if !ok {
			continue
		}
		parts = append(parts, ind.Label+" "+formatValue(ind, n))
		if len(parts) == 4 {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return period + " " + BasisLabel(basis) + "：" + strings.Join(parts, "，")
}

func formatValue(ind Indicator, n float64) string {
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if ind.Format == "number" {
		return s
	}
	return s + "%"
}

// Check reports a CPI-PPI row that does not match CPI minus PPI.
func Check(points []model.MacroPoint) []string {
	type pair struct{ cpi, ppi, spread *float64 }
	got := map[string]*pair{}
	for _, pt := range points {
		if pt.Value == nil {
			continue
		}
		ind, ok := Resolve(pt.Basis, pt.Indicator, 0)
		if !ok {
			continue
		}
		k := ind.Basis + "\t" + pt.Period
		slot := got[k]
		if slot == nil {
			slot = &pair{}
			got[k] = slot
		}
		switch ind.Key {
		case "cpi":
			slot.cpi = pt.Value
		case "ppi":
			slot.ppi = pt.Value
		case "cpi_ppi":
			slot.spread = pt.Value
		}
	}
	var warnings []string
	for key, slot := range got {
		if slot.cpi == nil || slot.ppi == nil || slot.spread == nil {
			continue
		}
		if math.Abs((*slot.cpi-*slot.ppi)-*slot.spread) <= 0.15 {
			continue
		}
		basis, period, _ := strings.Cut(key, "\t")
		warnings = append(warnings, period+" "+BasisLabel(basis)+" 的 CPI-PPI 和 CPI 减 PPI 不一致")
	}
	sort.Strings(warnings)
	return warnings
}
