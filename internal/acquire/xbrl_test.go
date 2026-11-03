package acquire

import (
	"testing"
	"time"
)

func TestQuarterAmountSplitsFiscalYear(t *testing.T) {
	start := time.Date(2025, 9, 28, 0, 0, 0, 0, time.UTC)
	facts := []FlowFact{
		{Start: start, End: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC), Val: 116.996e9},
		{Start: start, End: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC), Val: 145.097e9},
	}
	got, ok := QuarterAmount(facts, 2026, 4, 9)
	if !ok {
		t.Fatal("missing quarter")
	}
	billion := got / 1e9
	if billion < 28 || billion > 28.2 {
		t.Fatalf("q4 %v", billion)
	}
}
