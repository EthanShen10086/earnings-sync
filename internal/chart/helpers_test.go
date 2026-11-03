package chart

import "github.com/EthanShen10086/earnings-sync/internal/model"

func sample() model.Extraction {
	vals := map[string]float64{
		"total_revenue": 114.48, "iphone_revenue": 57.58, "services_revenue": 31.57,
		"net_income": 30.71, "capex": 2.2, "iphone_yoy": 0.17,
	}
	ex := model.Extraction{Ticker: "AAPL", Period: model.Period{FiscalYear: 2026, FiscalQuarter: 4}}
	for k, v := range vals {
		n := v
		ex.Facts = append(ex.Facts, model.Fact{LineItem: k, Value: &n})
	}
	return ex
}
