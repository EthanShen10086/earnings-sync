package sheet

import (
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"google.golang.org/api/sheets/v4"
)

// analysisCharts turn the latest quarter, and the whole history, into the questions
// the written reading is trying to answer. Formulas recalculate when a new quarter is appended.
var analysisCharts = []tmplChart{
	{Title: "硬件还是服务：本季收入从哪来", Sheet: "利润表", Type: model.ChartPie, DomainCol: 0, SeriesCol: 1, HeaderRow: 119, EndRow: 122, AnchorRow: 160, AnchorCol: 26},
	{Title: "收入能留下多少利润", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 3, SeriesCol: 4, HeaderRow: 119, EndRow: 122, AnchorRow: 180, AnchorCol: 26},
	{Title: "服务占收入的比例", Sheet: "利润表", Type: model.ChartColumn, DomainCol: 0, SeriesCol: 1, HeaderRow: 139, EndRow: 185, AnchorRow: 200, AnchorCol: 26},
	{Title: "家底和债务谁更大", Sheet: "资产负债表", Type: model.ChartColumn, DomainCol: 0, SeriesCol: 1, HeaderRow: 119, EndRow: 123, AnchorRow: 1, AnchorCol: 28},
	{Title: "存货和应收有没有胀", Sheet: "资产负债表", Type: model.ChartColumn, DomainCol: 3, SeriesCol: 4, HeaderRow: 119, EndRow: 122, AnchorRow: 22, AnchorCol: 28},
	{Title: "净现金盖不盖得住债", Sheet: "资产负债表", Type: model.ChartColumn, DomainCol: 0, SeriesCols: []int64{1, 2}, HeaderRow: 139, EndRow: 185, AnchorRow: 44, AnchorCol: 28},
	{Title: "这一季现金怎么来、怎么走", Sheet: "现金流量表", Type: model.ChartColumn, DomainCol: 0, SeriesCol: 1, HeaderRow: 119, EndRow: 124, AnchorRow: 42, AnchorCol: 17},
	{Title: "赚到的现金有没有还给股东", Sheet: "现金流量表", Type: model.ChartColumn, DomainCol: 0, SeriesCols: []int64{1, 2}, HeaderRow: 139, EndRow: 185, AnchorRow: 64, AnchorCol: 17},
}

func analysisFormulas() []*sheets.ValueRange {
	last := func(col string) string {
		return `IFERROR(INDEX(` + col + `:` + col + `,MAX(FILTER(ROW($A$2:$A$80),$A$2:$A$80<>""))),"")`
	}
	hardware := `IFERROR(` + last("D") + `+` + last("E") + `+` + last("F") + `+` + last("G") + `,"")`
	return []*sheets.ValueRange{
		{Range: "利润表!A120:B122", Values: [][]any{
			{"来源", "十亿美元"},
			{"硬件", "=" + hardware},
			{"服务", "=" + last("H")},
		}},
		{Range: "利润表!D120:E122", Values: [][]any{
			{"项目", "十亿美元"},
			{"总营收", "=" + last("J")},
			{"净利润", "=" + last("N")},
		}},
		{Range: "利润表!A140:B141", Values: [][]any{
			{"期间", "服务占收入"},
			{`=FILTER(C2:C80,A2:A80<>"")`, `=FILTER(ARRAYFORMULA(IFERROR(H2:H80/J2:J80)),A2:A80<>"")`},
		}},
		{Range: "资产负债表!A120:B123", Values: [][]any{
			{"项目", "十亿美元"},
			{"现金及证券", "=IFERROR(" + last("D") + "+" + last("E") + "+" + last("J") + `,"")`},
			{"有息债务", "=IFERROR(" + last("P") + "+" + last("T") + `,"")`},
			{"股东权益", "=" + last("X")},
		}},
		{Range: "资产负债表!D120:E122", Values: [][]any{
			{"项目", "十亿美元"},
			{"存货", "=" + last("G")},
			{"应收账款", "=" + last("F")},
		}},
		{Range: "资产负债表!A140:C141", Values: [][]any{
			{"期间", "净现金_十亿美元", "有息债务_十亿美元"},
			{
				`=FILTER(C2:C80,A2:A80<>"")`,
				`=FILTER(D2:D80+E2:E80+J2:J80-P2:P80-T2:T80,A2:A80<>"")`,
				`=FILTER(P2:P80+T2:T80,A2:A80<>"")`,
			},
		}},
		{Range: "现金流量表!A120:B124", Values: [][]any{
			{"项目", "十亿美元"},
			{"经营现金流", "=" + last("E")},
			{"资本开支", "=" + last("F")},
			{"自由现金流", "=" + last("G")},
			{"资本返还", "=" + last("J")},
		}},
		{Range: "现金流量表!A140:C141", Values: [][]any{
			{"期间", "经营现金流_十亿美元", "资本返还_十亿美元"},
			{
				`=FILTER(C2:C80,A2:A80<>"")`,
				`=FILTER(E2:E80,A2:A80<>"")`,
				`=FILTER(J2:J80,A2:A80<>"")`,
			},
		}},
	}
}
