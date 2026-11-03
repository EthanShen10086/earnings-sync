package agent

import (
	"context"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
)

type scripted struct {
	turns []extract.Turn
	n     int
}

func (s *scripted) Chat(context.Context, []extract.ChatMessage, []extract.ToolDef) (extract.Turn, extract.TokenUsage, error) {
	turn := s.turns[s.n]
	s.n++
	return turn, extract.TokenUsage{Total: 3}, nil
}

type notes struct{ n int }

func (n *notes) Send(context.Context, notify.Message) error {
	n.n++
	return nil
}

func TestAgentUsesToolsInsteadOfOnlyTalking(t *testing.T) {
	reg, err := config.Load("../../configs/companies.yaml")
	if err != nil {
		t.Fatal(err)
	}
	note := &notes{}
	model := &scripted{turns: []extract.Turn{
		{ToolCalls: []extract.ToolCall{{
			ID: "1", Name: "upsert_period",
			Arguments: `{"ticker":"AAPL","fiscal_year":2026,"fiscal_quarter":4,"facts":[{"line_item":"total_revenue","value":114.48}]}`,
		}}},
		{ToolCalls: []extract.ToolCall{{
			ID: "2", Name: "send_takeaway",
			Arguments: `{"ticker":"AAPL","period":"2026Q4","bullets":["服务收入继续抬高利润"]}`,
		}}},
		{Content: "已写入营收并准备通知。"},
	}}
	result, err := Run(context.Background(), model, Runtime{Companies: reg, Notifier: note, DryRun: true}, "导入这份财报")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 2 || result.Actions[0] != "upsert_period dry_run" {
		t.Fatalf("actions %#v", result.Actions)
	}
	if result.Notified || note.n != 0 {
		t.Fatal("dry run must not send")
	}
	if result.Extraction == nil || result.Extraction.Period.Label() != "2026Q4" {
		t.Fatalf("extraction %+v", result.Extraction)
	}
}
