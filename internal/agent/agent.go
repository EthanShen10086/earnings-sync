package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/chart"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/reconcile"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

const maxTurns = 6

const systemPrompt = `你是财报操作代理，不是只会回答的聊天机器人。
你必须通过工具完成动作。不要声称已经改了表格，除非工具返回成功。
可以做的事：查看已登记公司、把一季数字写入已有表格、新建三张报表、按规则生成图表、把 takeaway 发到飞书。
每家公司的字段和阅读口径以登记配置为准。配置列出了黄色格时，只写那些字段，不要套用别的公司的列。
怎么分析由财报正文和该公司自己的提示决定，不要套一套固定的行业结论。
没有 spreadsheet_id 时先 create_workbook，再 upsert_period。已经按自己的列登记过的公司不要新建空表。
金额用十亿美元。毛利率和同比用小数。不要发明工具名。
最后用一句中文说明你实际调用了哪些工具。`

// Chatter is the tool-calling side of a model provider.
type Chatter interface {
	Chat(ctx context.Context, messages []extract.ChatMessage, tools []extract.ToolDef) (extract.Turn, extract.TokenUsage, error)
}

// Runtime is the set of side effects the model is allowed to request.
type Runtime struct {
	Companies *config.Registry
	Sheets    sheet.Store
	Notifier  notify.Sender
	CredFile  string
	ShareWith string
	DryRun    bool
}

// Result is what the agent changed.
type Result struct {
	Actions    []string
	Takeaway   []string
	Warnings   []string
	Charts     []model.ChartSpec
	Cells      int
	Notified   bool
	SheetURL   string
	Tokens     int
	Extraction *model.Extraction
}

// Run lets the model call tools until it stops or the turn limit is reached.
func Run(ctx context.Context, model Chatter, rt Runtime, user string) (Result, error) {
	if model == nil {
		return Result{}, fmt.Errorf("没有可调用工具的模型")
	}
	tools := Definitions()
	messages := []extract.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: user},
	}
	var result Result
	for i := 0; i < maxTurns; i++ {
		turn, usage, err := model.Chat(ctx, messages, tools)
		if err != nil {
			return result, err
		}
		result.Tokens += usage.Total
		messages = append(messages, extract.ChatMessage{
			Role: "assistant", Content: turn.Content, Reasoning: turn.Reasoning, Calls: turn.ToolCalls,
		})
		if len(turn.ToolCalls) == 0 {
			if turn.Content != "" {
				result.Takeaway = append(result.Takeaway, turn.Content)
			}
			return result, nil
		}
		for _, call := range turn.ToolCalls {
			out, action := execute(ctx, rt, &result, call)
			if action != "" {
				result.Actions = append(result.Actions, action)
			}
			messages = append(messages, extract.ChatMessage{Role: "tool", ToolCall: call.ID, Content: out})
		}
	}
	return result, fmt.Errorf("模型连续调用工具超过 %d 轮，已停止", maxTurns)
}

func Definitions() []extract.ToolDef {
	facts := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"line_item": map[string]any{"type": "string"},
				"value":     map[string]any{"type": "number"},
				"unit":      map[string]any{"type": "string"},
				"text":      map[string]any{"type": "string"},
			},
			"required": []string{"line_item"},
		},
	}
	return []extract.ToolDef{
		{Name: "list_companies", Description: "列出已登记公司，以及是否已经有表格。不返回密钥或邮箱。", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "upsert_period", Description: "把一个财季的数字写入该公司已有表格的黄色录入格。没有表格时不要调用。", Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticker":         map[string]any{"type": "string"},
				"fiscal_year":    map[string]any{"type": "integer"},
				"fiscal_quarter": map[string]any{"type": "integer"},
				"facts":          facts,
			},
			"required": []string{"ticker", "fiscal_year", "fiscal_quarter", "facts"},
		}},
		{Name: "create_workbook", Description: "新建利润表、资产负债表、现金流量表，并把编辑权交给已配置的接收邮箱。", Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticker": map[string]any{"type": "string"},
				"title":  map[string]any{"type": "string"},
			},
			"required": []string{"ticker", "title"},
		}},
		{Name: "apply_chart_rules", Description: "按这份财季数据用规则生成或补齐柱状图和饼图。", Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticker":         map[string]any{"type": "string"},
				"fiscal_year":    map[string]any{"type": "integer"},
				"fiscal_quarter": map[string]any{"type": "integer"},
				"facts":          facts,
			},
			"required": []string{"ticker", "fiscal_year", "fiscal_quarter", "facts"},
		}},
		{Name: "send_takeaway", Description: "把需要关注的要点发到飞书。bullets 是 3 到 5 条短句。", Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticker":   map[string]any{"type": "string"},
				"period":   map[string]any{"type": "string"},
				"bullets":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"warnings": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"ticker", "period", "bullets"},
		}},
	}
}

func execute(ctx context.Context, rt Runtime, result *Result, call extract.ToolCall) (string, string) {
	var args map[string]any
	if call.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return err.Error(), call.Name + " invalid json"
		}
	}
	switch call.Name {
	case "list_companies":
		type row struct {
			Ticker   string `json:"ticker"`
			Name     string `json:"name"`
			HasSheet bool   `json:"has_sheet"`
			SheetURL string `json:"sheet_url,omitempty"`
			YearEnd  int    `json:"fiscal_year_end_month"`
		}
		var rows []row
		for _, c := range rt.Companies.List() {
			rows = append(rows, row{Ticker: c.Ticker, Name: c.Name, HasSheet: c.SpreadsheetID != "", SheetURL: c.SheetURL(), YearEnd: c.FiscalYearEndMonth})
		}
		b, _ := json.Marshal(rows)
		return string(b), "list_companies"
	case "upsert_period":
		ex := extractionFromArgs(args)
		company, ok := rt.Companies.Get(ex.Ticker)
		if ok {
			company.Bind(&ex)
		}
		result.Extraction = &ex
		result.Warnings = reconcile.Warnings(ex)
		if rt.DryRun {
			return `{"dry_run":true,"would_write":true}`, "upsert_period dry_run"
		}
		if !ok {
			return `{"error":"unknown ticker"}`, "upsert_period unknown"
		}
		if company.SpreadsheetID == "" || rt.Sheets == nil {
			return `{"error":"no spreadsheet; call create_workbook first"}`, "upsert_period no sheet"
		}
		begun := time.Now()
		n, err := rt.Sheets.Upsert(ctx, company.SpreadsheetID, ex)
		_ = begun
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), "upsert_period error"
		}
		result.Cells += n
		result.SheetURL = company.SheetURL()
		return fmt.Sprintf(`{"cells":%d,"sheet_url":%q}`, n, company.SheetURL()), "upsert_period"
	case "create_workbook":
		ticker, _ := args["ticker"].(string)
		title, _ := args["title"].(string)
		company, ok := rt.Companies.Get(ticker)
		if !ok {
			return `{"error":"unknown ticker"}`, "create_workbook unknown"
		}
		if company.UsesMappedLayout() {
			if company.SpreadsheetID != "" {
				return fmt.Sprintf(`{"spreadsheet_id":%q,"sheet_url":%q,"kept":true}`, company.SpreadsheetID, company.SheetURL()), "create_workbook kept mapped sheet"
			}
			return `{"error":"这家公司的黄色格已经在配置里列明，不能按苹果模板新建空表"}`, "create_workbook mapped"
		}
		if rt.DryRun {
			return `{"dry_run":true,"would_create":true}`, "create_workbook dry_run"
		}
		g, ok := rt.Sheets.(*sheet.Google)
		if !ok || g == nil {
			return `{"error":"google sheets client is not configured"}`, "create_workbook no client"
		}
		if title == "" {
			title = company.Name + " 财务报表"
		}
		id, err := g.CreateWorkbook(ctx, rt.CredFile, title, rt.ShareWith, company.Segments)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), "create_workbook error"
		}
		if err := rt.Companies.SetSpreadsheet(company.Ticker, id); err != nil {
			return fmt.Sprintf(`{"spreadsheet_id":%q,"warning":%q}`, id, err.Error()), "create_workbook unsaved"
		}
		company, _ = rt.Companies.Get(company.Ticker)
		result.SheetURL = company.SheetURL()
		return fmt.Sprintf(`{"spreadsheet_id":%q,"sheet_url":%q}`, id, company.SheetURL()), "create_workbook"
	case "apply_chart_rules":
		ex := extractionFromArgs(args)
		if company, ok := rt.Companies.Get(ex.Ticker); ok {
			company.Bind(&ex)
		}
		specs := chart.Recommend(ex)
		result.Charts = specs
		if rt.DryRun {
			return fmt.Sprintf(`{"dry_run":true,"charts":%d}`, len(specs)), "apply_chart_rules dry_run"
		}
		company, ok := rt.Companies.Get(ex.Ticker)
		if !ok || company.SpreadsheetID == "" || rt.Sheets == nil {
			return `{"error":"no spreadsheet"}`, "apply_chart_rules no sheet"
		}
		var n int
		var err error
		if g, ok := rt.Sheets.(*sheet.Google); ok {
			n, err = g.ApplyCompanyCharts(ctx, company.SpreadsheetID, specs, company.Segments, company.UsesMappedLayout())
		} else {
			n, err = rt.Sheets.ApplyCharts(ctx, company.SpreadsheetID, specs)
		}
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), "apply_chart_rules error"
		}
		return fmt.Sprintf(`{"charts_added":%d}`, n), "apply_chart_rules"
	case "send_takeaway":
		ticker, _ := args["ticker"].(string)
		period, _ := args["period"].(string)
		bullets := stringList(args["bullets"])
		warnings := stringList(args["warnings"])
		result.Takeaway = bullets
		if len(warnings) > 0 {
			result.Warnings = warnings
		}
		if rt.DryRun {
			return `{"dry_run":true}`, "send_takeaway dry_run"
		}
		if rt.Notifier == nil {
			return `{"error":"feishu is not configured"}`, "send_takeaway no notifier"
		}
		company, _ := rt.Companies.Get(ticker)
		err := rt.Notifier.Send(ctx, notify.Message{
			Ticker: ticker, Period: period, Takeaway: bullets, Warnings: result.Warnings, Charts: result.Charts, SheetURL: company.SheetURL(),
		})
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), "send_takeaway error"
		}
		result.Notified = true
		return `{"sent":true}`, "send_takeaway"
	default:
		return `{"error":"unknown tool"}`, call.Name + " unknown"
	}
}

// Call runs one tool by name. MCP and the model loop both use it.
func Call(ctx context.Context, rt Runtime, name, arguments string) (string, string) {
	var result Result
	return execute(ctx, rt, &result, extract.ToolCall{Name: name, Arguments: arguments})
}

func extractionFromArgs(args map[string]any) model.Extraction {
	ex := model.Extraction{
		Ticker: strings.ToUpper(fmt.Sprint(args["ticker"])),
		Period: model.Period{FiscalYear: asInt(args["fiscal_year"]), FiscalQuarter: asInt(args["fiscal_quarter"])},
	}
	raw, _ := args["facts"].([]any)
	for _, item := range raw {
		obj, _ := item.(map[string]any)
		if obj == nil {
			continue
		}
		name, _ := obj["line_item"].(string)
		if name == "" {
			continue
		}
		fact := model.Fact{LineItem: name, Statement: "income"}
		if text, _ := obj["text"].(string); text != "" {
			fact.Text = text
		}
		if v, ok := obj["value"].(float64); ok {
			n := v
			fact.Value = &n
		}
		ex.Facts = append(ex.Facts, fact)
	}
	return ex
}

func stringList(v any) []string {
	raw, _ := v.([]any)
	var out []string
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
