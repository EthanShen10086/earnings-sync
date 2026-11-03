package reconcile

import "testing"

func TestSegmentGap(t *testing.T) {
	ex := extraction(map[string]float64{
		"iphone_revenue":      57.58,
		"mac_revenue":         10.79,
		"ipad_revenue":        6.23,
		"wearables_revenue":   8.30,
		"services_revenue":    31.57,
		"total_revenue":       114.48,
		"operating_cash_flow": 28.10,
		"capex":               2.20,
		"free_cash_flow":      25.90,
		"gross_margin":        0.479,
	})
	warns := Warnings(ex)
	if len(warns) != 1 {
		t.Fatalf("warnings %#v", warns)
	}
}

func TestCleanCashFlow(t *testing.T) {
	ex := extraction(map[string]float64{
		"operating_cash_flow": 28.10,
		"capex":               2.20,
		"free_cash_flow":      25.90,
	})
	if len(Warnings(ex)) != 0 {
		t.Fatalf("%#v", Warnings(ex))
	}
}
