package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// StubProvider is a local model stand-in used when FEATURE_LLM=on without a real API key.
// It returns deterministic JSON so ingest / orchestrate paths stay executable.
type StubProvider struct {
	NameStr string
}

func (s StubProvider) Name() string {
	if s.NameStr != "" {
		return s.NameStr
	}
	return "stub-llm"
}

func (s StubProvider) Complete(_ context.Context, system, user string) (string, TokenUsage, error) {
	year, quarter := guessPeriod(user)
	rev := guessFloat(user, "revenue", 100)
	net := guessFloat(user, "net income", rev*0.25)
	payload := map[string]any{
		"periods": []map[string]any{{
			"fiscal_year":    year,
			"fiscal_quarter": quarter,
			"facts": []map[string]any{
				{"line_item": "total_revenue", "value": rev, "unit": "billion_usd"},
				{"line_item": "net_income", "value": net, "unit": "billion_usd"},
				{"line_item": "cash", "value": rev * 0.3, "unit": "billion_usd"},
				{"line_item": "equity", "value": rev * 0.8, "unit": "billion_usd"},
			},
		}},
		"analysis": []string{
			"好：stub 抽取命中营收与利润字段",
			"坏：无真实模型，数字为本地启发式",
			"结构：以 stub 固定科目为主",
			"赚钱：参考正文关键词估算",
			"负债表：现金与权益为派生值",
			"质地：仅用于无密钥联调",
		},
		"takeaway": []string{
			fmt.Sprintf("%dQ%d stub 营收约 %.2f", year, quarter, rev),
			fmt.Sprintf("stub 净利润约 %.2f", net),
			"本地 stub：请换 DEEPSEEK_API_KEY 走真模型",
		},
	}
	if strings.Contains(system, "takeaway") || strings.Contains(strings.ToLower(system), "买方") {
		payload = map[string]any{"takeaway": payload["takeaway"]}
	}
	raw, _ := json.Marshal(payload)
	return string(raw), TokenUsage{Total: 32}, nil
}

// StubChatter implements tool-calling without a remote model: it finishes with a short note.
type StubChatter struct {
	mu        sync.Mutex
	Calls     int
	LastTools []string
}

func (s *StubChatter) Chat(_ context.Context, _ []ChatMessage, tools []ToolDef) (Turn, TokenUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls++
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	s.LastTools = names
	return Turn{Content: "stub-llm：无真实 API key，跳过工具循环；结构化 CSV 仍可走完整入库路径。"}, TokenUsage{Total: 8}, nil
}

var (
	reYear    = regexp.MustCompile(`(?i)(?:fiscal[_\s-]?year|fy|年份)[^\d]{0,8}(20\d{2})`)
	reQuarter = regexp.MustCompile(`(?i)(?:fiscal[_\s-]?quarter|q|季度)[^\d]{0,6}([1-4])`)
)

func guessPeriod(user string) (year, quarter int) {
	year, quarter = 2026, 4
	if m := reYear.FindStringSubmatch(user); len(m) == 2 {
		if y, err := strconv.Atoi(m[1]); err == nil {
			year = y
		}
	}
	if m := reQuarter.FindStringSubmatch(user); len(m) == 2 {
		if q, err := strconv.Atoi(m[1]); err == nil {
			quarter = q
		}
	}
	return year, quarter
}

func guessFloat(user, label string, fallback float64) float64 {
	pat := `(?i)` + regexp.QuoteMeta(label) + `[^0-9\-]{0,12}(-?\d+(?:\.\d+)?)`
	re, err := regexp.Compile(pat)
	if err != nil {
		return fallback
	}
	if m := re.FindStringSubmatch(user); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v
		}
	}
	return fallback
}
