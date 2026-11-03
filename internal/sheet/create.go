package sheet

import (
	"context"
	"fmt"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// CreateWorkbook makes a three-statement spreadsheet owned by the authorized account
// and, when shareWith is set, gives that person edit access. The address is not logged.
func (g *Google) CreateWorkbook(ctx context.Context, credFile, title, shareWith string) (string, error) {
	if title == "" {
		title = "财务报表"
	}
	ss, err := g.svc.Spreadsheets.Create(&sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{Title: title},
		Sheets: []*sheets.Sheet{
			{Properties: &sheets.SheetProperties{Title: "利润表"}},
			{Properties: &sheets.SheetProperties{Title: "资产负债表"}},
			{Properties: &sheets.SheetProperties{Title: "现金流量表"}},
		},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("创建表格失败：%w。请确认这个 Cloud 项目已启用 Google Drive API。服务账号自己的云端硬盘没有容量时，需要把 GOOGLE_SHARE_WITH 设成你的邮箱，或改用用户授权", err)
	}
	_, err = g.svc.Spreadsheets.Values.BatchUpdate(ss.SpreadsheetId, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
		Data: []*sheets.ValueRange{
			{Range: "利润表!A1:Z1", Values: [][]any{incomeHeaders()}},
			{Range: "资产负债表!A1:AA1", Values: [][]any{balanceHeaders()}},
			{Range: "现金流量表!A1:P1", Values: [][]any{cashHeaders()}},
			{Range: "利润表!C2", Values: [][]any{{`=ARRAYFORMULA(IF(A2:A80="","",A2:A80&"Q"&B2:B80))`}}},
			{Range: "资产负债表!C2", Values: [][]any{{`=ARRAYFORMULA(IF(A2:A80="","",A2:A80&"Q"&B2:B80))`}}},
			{Range: "现金流量表!C2", Values: [][]any{{`=ARRAYFORMULA(IF(A2:A80="","",A2:A80&"Q"&B2:B80))`}}},
		},
	}).Context(ctx).Do()
	if err != nil {
		return ss.SpreadsheetId, fmt.Errorf("表已创建但表头写入失败：%w", err)
	}
	if shareWith != "" {
		if err := shareEdit(ctx, credFile, ss.SpreadsheetId, shareWith); err != nil {
			return ss.SpreadsheetId, err
		}
	}
	return ss.SpreadsheetId, nil
}

func shareEdit(ctx context.Context, credFile, spreadsheetID, email string) error {
	drv, err := drive.NewService(ctx, option.WithCredentialsFile(credFile), option.WithScopes("https://www.googleapis.com/auth/drive.file"))
	if err != nil {
		return err
	}
	_, err = drv.Permissions.Create(spreadsheetID, &drive.Permission{
		Type:         "user",
		Role:         "writer",
		EmailAddress: email,
	}).SendNotificationEmail(false).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("表格已创建，但还不能共享给你填写的邮箱：%w。请确认已启用 Google Drive API", err)
	}
	return nil
}

func incomeHeaders() []any {
	return []any{"财年", "财季", "期间", "iPhone营收_十亿美元", "Mac营收_十亿美元", "iPad营收_十亿美元", "可穿戴家居配件营收_十亿美元", "服务营收_十亿美元", "分部合计_十亿美元", "总营收_十亿美元", "分部勾稽差额_十亿美元", "毛利率", "毛利_十亿美元", "净利润_十亿美元", "稀释每股收益_美元", "iPhone出货量_百万台", "出货量口径", "总营收同比", "净利润同比", "毛利率变动_百分点", "iPhone披露同比", "Mac披露同比", "iPad披露同比", "可穿戴披露同比", "服务披露同比", "备注"}
}

func balanceHeaders() []any {
	return []any{"财年", "财季", "期间", "现金及现金等价物_十亿美元", "短期有价证券_十亿美元", "应收账款_十亿美元", "存货_十亿美元", "其他流动资产_十亿美元", "流动资产合计_十亿美元", "长期有价证券_十亿美元", "不动产厂房及设备净值_十亿美元", "其他非流动资产_十亿美元", "非流动资产合计_十亿美元", "资产总计_十亿美元", "应付账款_十亿美元", "短期借款及商业票据_十亿美元", "递延收入_十亿美元", "其他流动负债_十亿美元", "流动负债合计_十亿美元", "长期债务_十亿美元", "其他非流动负债_十亿美元", "非流动负债合计_十亿美元", "负债合计_十亿美元", "股东权益_十亿美元", "负债及股东权益总计_十亿美元", "平衡差额_十亿美元", "备注"}
}

func cashHeaders() []any {
	return []any{"财年", "财季", "期间", "净利润_十亿美元", "经营活动现金流净额_十亿美元", "资本开支_十亿美元", "自由现金流_披露_十亿美元", "自由现金流_十亿美元", "自由现金流勾稽差额_十亿美元", "资本返还_十亿美元", "每股股息_美元", "折旧与摊销_十亿美元", "其他经营调整_十亿美元", "其他投资现金流_十亿美元", "其他筹资现金流_十亿美元", "备注"}
}
