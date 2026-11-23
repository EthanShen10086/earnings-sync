package sheet

import (
	"context"
	"fmt"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// CreateWorkbook makes a three-statement spreadsheet owned by the authorized account
// and, when shareWith is set, gives that person edit access. The address is not logged.
func (g *Google) CreateWorkbook(ctx context.Context, credFile, title, shareWith string, segments []config.Segment) (string, error) {
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
		msg := err.Error()
		if strings.Contains(msg, "storageQuotaExceeded") || strings.Contains(msg, "does not have storage") {
			return "", fmt.Errorf("创建表格失败：服务账号云端硬盘配额为 0。请用自己的 Google 账号新建三张表（利润表/资产负债表/现金流量表），共享给服务账号 JSON 里的 client_email（编辑者），再把 spreadsheet id 写进 configs/companies.yaml。原始错误：%w", err)
		}
		return "", fmt.Errorf("创建表格失败：%w。请确认这个 Cloud 项目已启用 Google Drive API。服务账号自己的云端硬盘没有容量时，需要把 GOOGLE_SHARE_WITH 设成你的邮箱，或改用用户授权", err)
	}
	if err := g.writeWorkbookScaffold(ctx, ss.SpreadsheetId, segments); err != nil {
		return ss.SpreadsheetId, err
	}
	if shareWith != "" {
		if err := shareEdit(ctx, credFile, ss.SpreadsheetId, shareWith); err != nil {
			return ss.SpreadsheetId, err
		}
	}
	return ss.SpreadsheetId, nil
}

// EnsureWorkbook prepares an existing spreadsheet for quarterly ingest.
// The first tab (gid/index 0) becomes 利润表; 资产负债表 and 现金流量表 are added if missing.
// Headers and period formulas are written. It does not create a new spreadsheet file.
func (g *Google) EnsureWorkbook(ctx context.Context, spreadsheetID string, segments []config.Segment) error {
	if spreadsheetID == "" {
		return fmt.Errorf("spreadsheet id 为空")
	}
	ss, err := g.svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(sheetId,title,index))").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("打不开表格：%w。把表格共享给服务账号 JSON 里的 client_email，权限选编辑者", err)
	}
	if len(ss.Sheets) == 0 {
		return fmt.Errorf("表格没有任何工作表")
	}
	byTitle := map[string]*sheets.SheetProperties{}
	var first *sheets.SheetProperties
	for _, sh := range ss.Sheets {
		p := sh.Properties
		byTitle[p.Title] = p
		if p.Index == 0 {
			first = p
		}
	}
	if first == nil {
		first = ss.Sheets[0].Properties
	}
	var reqs []*sheets.Request
	if _, ok := byTitle["利润表"]; !ok {
		reqs = append(reqs, &sheets.Request{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{SheetId: first.SheetId, Title: "利润表"},
			Fields:     "title",
		}})
		delete(byTitle, first.Title)
		byTitle["利润表"] = first
	}
	for _, name := range []string{"资产负债表", "现金流量表"} {
		if _, ok := byTitle[name]; ok {
			continue
		}
		reqs = append(reqs, &sheets.Request{AddSheet: &sheets.AddSheetRequest{
			Properties: &sheets.SheetProperties{Title: name},
		}})
	}
	if len(reqs) > 0 {
		_, err = g.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: reqs}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("整理工作表失败：%w", err)
		}
	}
	return g.writeWorkbookScaffold(ctx, spreadsheetID, segments)
}

func (g *Google) writeWorkbookScaffold(ctx context.Context, spreadsheetID string, segments []config.Segment) error {
	_, err := g.svc.Spreadsheets.Values.BatchUpdate(spreadsheetID, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
		Data:             workbookValues(segments),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("表头写入失败：%w", err)
	}
	return nil
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

func workbookValues(segments []config.Segment) []*sheets.ValueRange {
	data := []*sheets.ValueRange{
		{Range: "利润表!A1:Z1", Values: [][]any{incomeHeaders(segments)}},
		{Range: "资产负债表!A1:AA1", Values: [][]any{balanceHeaders()}},
		{Range: "现金流量表!A1:P1", Values: [][]any{cashHeaders()}},
		{Range: "利润表!C2", Values: [][]any{{`=ARRAYFORMULA(IF(A2:A80="","",A2:A80&"Q"&B2:B80))`}}},
		{Range: "资产负债表!C2", Values: [][]any{{`=ARRAYFORMULA(IF(A2:A80="","",A2:A80&"Q"&B2:B80))`}}},
		{Range: "现金流量表!C2", Values: [][]any{{`=ARRAYFORMULA(IF(A2:A80="","",A2:A80&"Q"&B2:B80))`}}},
	}
	if formula := segmentSumFormula(segments); formula != "" {
		data = append(data, &sheets.ValueRange{Range: "利润表!I2", Values: [][]any{{formula}}})
	}
	return data
}

func segmentSumFormula(segments []config.Segment) string {
	if len(segments) == 0 {
		return ""
	}
	parts := make([]string, 0, len(segments))
	for _, s := range segments {
		col := strings.ToUpper(strings.TrimSpace(s.Column))
		if col == "" {
			continue
		}
		parts = append(parts, col+"2:"+col+"80")
	}
	if len(parts) == 0 {
		return ""
	}
	return `=ARRAYFORMULA(IF(A2:A80="","",` + strings.Join(parts, "+") + `))`
}

func incomeHeaders(segments []config.Segment) []any {
	row := []any{"财年", "财季", "期间", "iPhone营收_十亿美元", "Mac营收_十亿美元", "iPad营收_十亿美元", "可穿戴家居配件营收_十亿美元", "服务营收_十亿美元", "分部合计_十亿美元", "总营收_十亿美元", "分部勾稽差额_十亿美元", "毛利率", "毛利_十亿美元", "净利润_十亿美元", "稀释每股收益_美元", "iPhone出货量_百万台", "出货量口径", "总营收同比", "净利润同比", "毛利率变动_百分点", "iPhone披露同比", "Mac披露同比", "iPad披露同比", "可穿戴披露同比", "服务披露同比", "备注"}
	if len(segments) == 0 {
		return row
	}
	for _, i := range []int{3, 4, 5, 6, 7, 15, 16, 20, 21, 22, 23, 24} {
		row[i] = ""
	}
	for _, s := range segments {
		idx := colIndex(s.Column)
		if idx >= 0 && idx < len(row) {
			row[idx] = s.Header
		}
	}
	return row
}

func colIndex(col string) int {
	col = strings.ToUpper(strings.TrimSpace(col))
	n := 0
	for _, r := range col {
		if r < 'A' || r > 'Z' {
			return -1
		}
		n = n*26 + int(r-'A'+1)
	}
	return n - 1
}

func balanceHeaders() []any {
	return []any{"财年", "财季", "期间", "现金及现金等价物_十亿美元", "短期有价证券_十亿美元", "应收账款_十亿美元", "存货_十亿美元", "其他流动资产_十亿美元", "流动资产合计_十亿美元", "长期有价证券_十亿美元", "不动产厂房及设备净值_十亿美元", "其他非流动资产_十亿美元", "非流动资产合计_十亿美元", "资产总计_十亿美元", "应付账款_十亿美元", "短期借款及商业票据_十亿美元", "递延收入_十亿美元", "其他流动负债_十亿美元", "流动负债合计_十亿美元", "长期债务_十亿美元", "其他非流动负债_十亿美元", "非流动负债合计_十亿美元", "负债合计_十亿美元", "股东权益_十亿美元", "负债及股东权益总计_十亿美元", "平衡差额_十亿美元", "备注"}
}

func cashHeaders() []any {
	return []any{"财年", "财季", "期间", "净利润_十亿美元", "经营活动现金流净额_十亿美元", "资本开支_十亿美元", "自由现金流_披露_十亿美元", "自由现金流_十亿美元", "自由现金流勾稽差额_十亿美元", "资本返还_十亿美元", "每股股息_美元", "折旧与摊销_十亿美元", "其他经营调整_十亿美元", "其他投资现金流_十亿美元", "其他筹资现金流_十亿美元", "备注"}
}
