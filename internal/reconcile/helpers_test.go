package reconcile

import "github.com/EthanShen10086/earnings-sync/internal/model"

func extraction(vals map[string]float64) model.Extraction {
	ex := model.Extraction{Period: model.Period{FiscalYear: 2026, FiscalQuarter: 4}}
	for k, v := range vals {
		n := v
		ex.Facts = append(ex.Facts, model.Fact{LineItem: k, Value: &n})
	}
	return ex
}
