package canary_test

import (
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/canary"
)

func TestCanaryStateMachine(t *testing.T) {
	c := canary.NewController()
	r, err := c.Start("rel-1", "earnings-ingest", "v1", "v2", "acme")
	if err != nil || r.Phase != canary.PhaseDraft {
		t.Fatalf("start: %+v err=%v", r, err)
	}

	ver, bucket, err := c.PickVersion("rel-1", "user-a")
	if err != nil || bucket != "stable" || ver != "v1" {
		t.Fatalf("draft pick ver=%s bucket=%s err=%v", ver, bucket, err)
	}

	if _, err := c.SetCanary("rel-1", 100); err != nil {
		t.Fatal(err)
	}
	ver, bucket, err = c.PickVersion("rel-1", "user-b")
	if err != nil || bucket != "canary" || ver != "v2" {
		t.Fatalf("canary100 ver=%s bucket=%s err=%v", ver, bucket, err)
	}

	if _, err := c.Promote("rel-1"); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get("rel-1")
	if !ok || got.Phase != canary.PhasePromoted || got.StableVer != "v2" {
		t.Fatalf("promoted: %+v", got)
	}
}

func TestRollback(t *testing.T) {
	c := canary.NewController()
	_, _ = c.Start("r2", "flow", "a", "b", "")
	_, _ = c.SetCanary("r2", 50)
	r, err := c.Rollback("r2")
	if err != nil || r.Phase != canary.PhaseRolledBack || r.Percent != 0 {
		t.Fatalf("rollback: %+v err=%v", r, err)
	}
}
