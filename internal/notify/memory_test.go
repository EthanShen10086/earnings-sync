package notify

import (
	"context"
	"testing"
)

func TestMemorySender(t *testing.T) {
	m := &Memory{}
	if err := m.Send(context.Background(), Message{Ticker: "AAPL", Period: "2026Q4", Takeaway: []string{"ok"}}); err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1 {
		t.Fatal(m.Len())
	}
	last, ok := m.Last()
	if !ok || last.Ticker != "AAPL" || len(last.Takeaway) != 1 {
		t.Fatalf("%+v", last)
	}
}
