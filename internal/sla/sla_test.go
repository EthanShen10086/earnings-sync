package sla_test

import (
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/sla"
)

func TestBackupDrillHooks(t *testing.T) {
	b := sla.NewBoard()
	d, err := b.ScheduleDrill(sla.Drill{
		ID:        "drill-1",
		Kind:      "restore",
		TenantID:  "acme",
		TargetRTO: 10 * time.Minute,
	})
	if err != nil || d.Phase != sla.DrillScheduled {
		t.Fatalf("schedule: %+v err=%v", d, err)
	}

	d, err = b.RunDrill("drill-1", 2*time.Minute)
	if err != nil || d.Phase != sla.DrillPassed {
		t.Fatalf("run: %+v err=%v", d, err)
	}
	if len(d.Hooks) != 3 {
		t.Fatalf("hooks=%v", d.Hooks)
	}

	d2, _ := b.ScheduleDrill(sla.Drill{ID: "drill-2", Kind: "backup", TargetRTO: time.Minute})
	d2, err = b.RunDrill("drill-2", 5*time.Minute)
	if err != nil || d2.Phase != sla.DrillFailed {
		t.Fatalf("want RTO fail: %+v err=%v", d2, err)
	}

	b.RecordCheck(sla.Check{Name: "ingest_p99", OK: true, Latency: 20 * time.Millisecond})
	if len(b.RecentChecks(1)) != 1 {
		t.Fatal("checks")
	}
}
