package reconcile

import (
	"fmt"
	"math"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Warnings compares related facts. Small rounding stays visible; it does not fail the job.
func Warnings(ex model.Extraction) []string {
	var out []string
	segments := []string{"iphone_revenue", "mac_revenue", "ipad_revenue", "wearables_revenue", "services_revenue"}
	sum := 0.0
	n := 0
	for _, item := range segments {
		if v, ok := ex.Number(item); ok {
			sum += v
			n++
		}
	}
	if total, ok := ex.Number("total_revenue"); ok && n == len(segments) {
		diff := round2(total - sum)
		if diff != 0 {
			out = append(out, fmt.Sprintf("分部合计 %.2f 与总营收 %.2f 相差 %.2f 十亿美元", sum, total, diff))
		}
	}
	ocf, hasOCF := ex.Number("operating_cash_flow")
	capex, hasCapex := ex.Number("capex")
	fcf, hasFCF := ex.Number("free_cash_flow")
	if hasOCF && hasCapex && hasFCF {
		diff := round2((ocf - capex) - fcf)
		if diff != 0 {
			out = append(out, fmt.Sprintf("经营现金流减资本开支为 %.2f，披露自由现金流 %.2f，相差 %.2f", ocf-capex, fcf, diff))
		}
	}
	if gm, ok := ex.Number("gross_margin"); ok && (gm < 0 || gm > 1) {
		out = append(out, fmt.Sprintf("毛利率 %.4f 不在 0 到 1 之间", gm))
	}
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
