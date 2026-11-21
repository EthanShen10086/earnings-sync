package extract

import (
	"context"
	"strings"
	"testing"
)

func TestStubProviderComplete(t *testing.T) {
	p := StubProvider{}
	raw, usage, err := p.Complete(context.Background(), extractSystem, "fiscal_year 2025 fiscal_quarter 2 revenue 88.5 net income 20")
	if err != nil {
		t.Fatal(err)
	}
	if usage.Total == 0 {
		t.Fatal("expected token usage")
	}
	if !strings.Contains(raw, "total_revenue") || !strings.Contains(raw, "2025") {
		t.Fatalf("unexpected stub payload: %s", raw)
	}
	ex, err := decodeModel(StripFence(raw), "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Period.FiscalYear != 2025 || ex.Period.FiscalQuarter != 2 {
		t.Fatalf("period %+v", ex.Period)
	}
}

func TestStubChatter(t *testing.T) {
	c := &StubChatter{}
	turn, _, err := c.Chat(context.Background(), nil, []ToolDef{{Name: "list_companies"}})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Content == "" || c.Calls != 1 {
		t.Fatalf("turn=%q calls=%d", turn.Content, c.Calls)
	}
}
