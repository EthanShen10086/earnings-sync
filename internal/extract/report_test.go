package extract

import "testing"

func TestDecodeReportKeepsBalanceSheet(t *testing.T) {
	raw := `{"periods":[{"fiscal_year":2026,"fiscal_quarter":4,"facts":[
		{"line_item":"total_revenue","value":114477,"unit":"million_usd"},
		{"line_item":"gross_margin","value":54.82},
		{"line_item":"net_income","value":30.711},
		{"line_item":"cash","value":36305,"unit":"million_usd"},
		{"line_item":"receivables","value":43.817},
		{"line_item":"inventory","value":14.541},
		{"line_item":"accounts_payable","value":82.252},
		{"line_item":"long_term_debt","value":68.208},
		{"line_item":"equity","value":112.093}
	]}],"analysis":["好：iPhone 创季度纪录","负债表：现金高于短期债务"]}`
	periods, analysis, err := decodeReport(raw, "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if periods[0].Period.Label() != "2026Q4" || len(analysis) != 2 {
		t.Fatalf("%+v %v", periods[0].Period, analysis)
	}
	cash, _ := periods[0].Number("cash")
	rev, _ := periods[0].Number("total_revenue")
	if cash < 36 || cash > 37 || rev < 114 || rev > 115 {
		t.Fatalf("cash %v revenue %v", cash, rev)
	}
	if len(MissingCore(periods[0])) != 0 {
		t.Fatal(MissingCore(periods[0]))
	}
	gm, _ := periods[0].Number("gross_margin")
	if gm < 0.47 || gm > 0.49 {
		t.Fatalf("gross margin %v", gm)
	}
}
