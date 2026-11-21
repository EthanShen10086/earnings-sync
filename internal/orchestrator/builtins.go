package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/extract"
)

// BuiltinTools returns demo / earnings-oriented tools that need no external APIs.
// Real sheet/notify side effects stay in internal/agent; these keep the engine runnable offline.
func BuiltinTools() ToolRegistry {
	return ToolRegistry{
		"echo": func(_ context.Context, input any) (any, error) {
			return map[string]any{"echo": input}, nil
		},
		"validate_ingest": func(_ context.Context, input any) (any, error) {
			m, _ := input.(map[string]any)
			ticker := strings.ToUpper(fmt.Sprint(m["ticker"]))
			if ticker == "" || ticker == "<nil>" {
				return nil, fmt.Errorf("ticker 不能为空")
			}
			dry := false
			if v, ok := m["dry_run"]; ok {
				dry, _ = asBool(v)
			}
			return map[string]any{
				"ticker":  ticker,
				"dry_run": dry,
				"ok":      true,
			}, nil
		},
		"extract_facts_stub": func(_ context.Context, input any) (any, error) {
			m, _ := input.(map[string]any)
			ticker := strings.ToUpper(fmt.Sprint(m["ticker"]))
			return map[string]any{
				"ticker": ticker,
				"period": map[string]any{"fiscal_year": 2026, "fiscal_quarter": 4},
				"facts": []any{
					map[string]any{"line_item": "revenue", "value": 100.0, "unit": "billion_usd"},
					map[string]any{"line_item": "net_income", "value": 25.0, "unit": "billion_usd"},
				},
				"takeaway": []string{
					ticker + " 营收与净利已从样例输入抽出（stub，未调用模型）",
				},
			}, nil
		},
		"reconcile_stub": func(_ context.Context, input any) (any, error) {
			m, _ := input.(map[string]any)
			return map[string]any{
				"warnings": []any{},
				"ok":       true,
				"source":   m,
			}, nil
		},
		"upsert_period_stub": func(_ context.Context, input any) (any, error) {
			m, _ := input.(map[string]any)
			return map[string]any{
				"cells":     12,
				"written":   true,
				"ticker":    m["ticker"],
				"sheet_url": "https://example.invalid/sheet/" + fmt.Sprint(m["ticker"]),
			}, nil
		},
		"notify_stub": func(_ context.Context, input any) (any, error) {
			m, _ := input.(map[string]any)
			return map[string]any{
				"sent":   true,
				"ticker": m["ticker"],
				"period": m["period"],
			}, nil
		},
	}
}

// MockChatter returns a fixed assistant turn without calling a remote model.
type MockChatter struct {
	Content string
}

func (m MockChatter) Chat(ctx context.Context, messages []extract.ChatMessage, tools []extract.ToolDef) (extract.Turn, extract.TokenUsage, error) {
	_ = ctx
	_ = messages
	_ = tools
	content := m.Content
	if content == "" {
		content = "mock agent 完成"
	}
	return extract.Turn{Content: content}, extract.TokenUsage{}, nil
}
