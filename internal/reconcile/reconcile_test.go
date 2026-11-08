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

func TestConfiguredSegmentGap(t *testing.T) {
	ex := extraction(map[string]float64{
		"productivity_revenue":      20,
		"intelligent_cloud_revenue": 30,
		"total_revenue":             55,
	})
	ex.Columns = map[string]string{"productivity_revenue": "D", "intelligent_cloud_revenue": "E"}
	warns := Warnings(ex)
	if len(warns) != 1 {
		t.Fatalf("%#v", warns)
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

func TestOperatingSegmentsReconcileToDisclosedTotal(t *testing.T) {
	ex := extraction(map[string]float64{
		"insurance_underwriting":       1.352,
		"insurance_investment_income":  2.734,
		"bnsf_earnings":                1.729,
		"bhe_earnings":                 1.612,
		"msr_earnings":                 4.042,
		"other_operating":              1.236,
		"disclosed_operating_earnings": 12.705,
		"total_revenue":                105.647,
	})
	ex.Columns = map[string]string{
		"insurance_underwriting": "D", "insurance_investment_income": "E",
		"bnsf_earnings": "F", "bhe_earnings": "G", "msr_earnings": "H", "other_operating": "I",
	}
	ex.SegmentTotal = "disclosed_operating_earnings"
	if len(Warnings(ex)) != 0 {
		t.Fatalf("should tie to operating earnings, got %#v", Warnings(ex))
	}
}
