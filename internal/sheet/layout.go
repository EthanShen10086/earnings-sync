package sheet

import (
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// inputColumns are the yellow cells. Formula columns are intentionally absent.
var inputColumns = map[string]cellRef{
	"iphone_revenue":               {Sheet: "利润表", Col: "D"},
	"mac_revenue":                  {Sheet: "利润表", Col: "E"},
	"ipad_revenue":                 {Sheet: "利润表", Col: "F"},
	"wearables_revenue":            {Sheet: "利润表", Col: "G"},
	"services_revenue":             {Sheet: "利润表", Col: "H"},
	"total_revenue":                {Sheet: "利润表", Col: "J"},
	"gross_margin":                 {Sheet: "利润表", Col: "L"},
	"net_income":                   {Sheet: "利润表", Col: "N"},
	"diluted_eps":                  {Sheet: "利润表", Col: "O"},
	"iphone_units":                 {Sheet: "利润表", Col: "P"},
	"units_source":                 {Sheet: "利润表", Col: "Q"},
	"iphone_yoy":                   {Sheet: "利润表", Col: "U"},
	"mac_yoy":                      {Sheet: "利润表", Col: "V"},
	"ipad_yoy":                     {Sheet: "利润表", Col: "W"},
	"wearables_yoy":                {Sheet: "利润表", Col: "X"},
	"services_yoy":                 {Sheet: "利润表", Col: "Y"},
	"income_note":                  {Sheet: "利润表", Col: "Z"},
	"cash":                         {Sheet: "资产负债表", Col: "D"},
	"short_term_investments":       {Sheet: "资产负债表", Col: "E"},
	"receivables":                  {Sheet: "资产负债表", Col: "F"},
	"inventory":                    {Sheet: "资产负债表", Col: "G"},
	"other_current_assets":         {Sheet: "资产负债表", Col: "H"},
	"long_term_investments":        {Sheet: "资产负债表", Col: "J"},
	"ppe_net":                      {Sheet: "资产负债表", Col: "K"},
	"other_noncurrent_assets":      {Sheet: "资产负债表", Col: "L"},
	"accounts_payable":             {Sheet: "资产负债表", Col: "O"},
	"short_term_debt":              {Sheet: "资产负债表", Col: "P"},
	"deferred_revenue":             {Sheet: "资产负债表", Col: "Q"},
	"other_current_liabilities":    {Sheet: "资产负债表", Col: "R"},
	"long_term_debt":               {Sheet: "资产负债表", Col: "T"},
	"other_noncurrent_liabilities": {Sheet: "资产负债表", Col: "U"},
	"equity":                       {Sheet: "资产负债表", Col: "X"},
	"balance_note":                 {Sheet: "资产负债表", Col: "AA"},
	"operating_cash_flow":          {Sheet: "现金流量表", Col: "E"},
	"capex":                        {Sheet: "现金流量表", Col: "F"},
	"free_cash_flow":               {Sheet: "现金流量表", Col: "G"},
	"capital_return":               {Sheet: "现金流量表", Col: "J"},
	"dividend_per_share":           {Sheet: "现金流量表", Col: "K"},
	"depreciation":                 {Sheet: "现金流量表", Col: "L"},
	"other_operating":              {Sheet: "现金流量表", Col: "M"},
	"other_investing":              {Sheet: "现金流量表", Col: "N"},
	"other_financing":              {Sheet: "现金流量表", Col: "O"},
	"cashflow_note":                {Sheet: "现金流量表", Col: "P"},
	"note":                         {Sheet: "利润表", Col: "Z"},
}

type cellRef struct {
	Sheet string
	Col   string
}

// templateCharts match the quarterly workbook already used for Apple.
var templateCharts = []tmplChart{
	{Title: "总营收（已披露期间）", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 0, SeriesCol: 1, HeaderRow: 83, EndRow: 103, AnchorRow: 1, AnchorCol: 26},
	{Title: "净利润（已披露期间）", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 3, SeriesCol: 4, HeaderRow: 83, EndRow: 103, AnchorRow: 21, AnchorCol: 26},
	{Title: "毛利率（已披露期间）", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 6, SeriesCol: 7, HeaderRow: 83, EndRow: 103, AnchorRow: 41, AnchorCol: 26},
	{Title: "iPhone营收（已披露期间）", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 9, SeriesCol: 10, HeaderRow: 83, EndRow: 103, AnchorRow: 61, AnchorCol: 26},
	{Title: "服务营收（已披露期间）", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 12, SeriesCol: 13, HeaderRow: 83, EndRow: 103, AnchorRow: 81, AnchorCol: 26},
	{Title: "iPhone出货量（已披露期间，百万台）", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 15, SeriesCol: 16, HeaderRow: 83, EndRow: 103, AnchorRow: 101, AnchorCol: 26},
	{Title: "所选季度营收构成", Sheet: "利润表", Type: model.ChartPie, DomainCol: 18, SeriesCol: 19, HeaderRow: 83, EndRow: 89, AnchorRow: 121, AnchorCol: 26},
	{Title: "所选季度分业务同比", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 21, SeriesCol: 22, HeaderRow: 83, EndRow: 89, AnchorRow: 141, AnchorCol: 26},
	{Title: "资本开支（已披露期间）", Sheet: "现金流量表", Type: model.ChartColumn, DomainCol: 0, SeriesCol: 1, HeaderRow: 83, EndRow: 110, AnchorRow: 1, AnchorCol: 17},
	{Title: "自由现金流（已披露期间）", Sheet: "现金流量表", Type: model.ChartColumn, DomainCol: 3, SeriesCol: 4, HeaderRow: 83, EndRow: 110, AnchorRow: 21, AnchorCol: 17},
}

type tmplChart struct {
	Title      string
	Sheet      string
	Type       string
	DomainCol  int64
	SeriesCol  int64
	HeaderRow  int64
	EndRow     int64
	AnchorRow  int64
	AnchorCol  int64
	SeriesCols []int64
}

func refFor(item string, columns map[string]string) (cellRef, bool) {
	if col, ok := columns[item]; ok && col != "" {
		return cellRef{Sheet: "利润表", Col: strings.ToUpper(col)}, true
	}
	ref, ok := inputColumns[item]
	return ref, ok
}

func templateByTitle(title string) (tmplChart, bool) {
	for _, c := range templateCharts {
		if c.Title == title {
			return c, true
		}
	}
	for _, c := range analysisCharts {
		if c.Title == title {
			return c, true
		}
	}
	return tmplChart{}, false
}
