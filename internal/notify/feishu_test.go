package notify

import "testing"

func TestSignStable(t *testing.T) {
	got := Sign("1599360473", "secret")
	if got == "" || Sign("1599360473", "secret") != got {
		t.Fatal(got)
	}
	if Sign("1599360473", "other") == got {
		t.Fatal("signature ignored the secret")
	}
}

func TestMessageStartsWithTakeaway(t *testing.T) {
	text := Message{
		Ticker:   "AAPL",
		Period:   "2026Q4",
		Takeaway: []string{"服务收入继续抬高毛利率"},
		Warnings: nil,
		SheetURL: "https://docs.google.com/spreadsheets/d/abc",
	}.Text()
	if text[:12] == "" || !contains(text, "解读") || !contains(text, "服务收入继续抬高毛利率") {
		t.Fatal(text)
	}
	if !contains(text, "未发现勾稽差异") || !contains(text, "/abc") {
		t.Fatal(text)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
