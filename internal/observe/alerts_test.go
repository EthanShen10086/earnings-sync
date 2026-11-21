package observe

import "testing"

func TestAlertBoardListAndUpsert(t *testing.T) {
	b := NewAlertBoard()
	if len(b.List()) < 1 {
		t.Fatal("expected stub alerts")
	}
	b.Upsert(Alert{ID: "demo-ingest-latency", Severity: "warn", Title: "updated", Source: "stub", Firing: true})
	found := false
	for _, a := range b.List() {
		if a.ID == "demo-ingest-latency" && a.Firing && a.Title == "updated" {
			found = true
		}
	}
	if !found {
		t.Fatal("upsert failed")
	}
}
