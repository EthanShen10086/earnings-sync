package agent

import (
	"context"
	"os"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
)

func TestLiveToolCall(t *testing.T) {
	if os.Getenv("EARNINGS_LIVE") == "" {
		t.Skip("设置 EARNINGS_LIVE=1 才请求真实 DeepSeek")
	}
	config.LoadDotEnv("../../.env")
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Fatal("DEEPSEEK_API_KEY 为空")
	}
	model := extract.HTTPProvider{
		BaseURL: env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
		Model:   env("DEEPSEEK_MODEL", "deepseek-flash"),
	}
	turn, _, err := model.Chat(context.Background(), []extract.ChatMessage{{
		Role: "user", Content: "请调用 list_companies，不要直接回答。",
	}}, []extract.ToolDef{{
		Name: "list_companies", Description: "列出公司", Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.ToolCalls) == 0 || turn.ToolCalls[0].Name != "list_companies" {
		t.Fatalf("模型 %s 没有调用工具：%q", model.AgentModel(), turn.Content)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
