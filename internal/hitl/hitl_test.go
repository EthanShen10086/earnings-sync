package hitl_test

import (
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/hitl"
)

func TestInterventionRing(t *testing.T) {
	ring := hitl.NewRing()
	req, err := ring.Ask(hitl.Request{
		ID:     "iv-1",
		RunID:  "run-9",
		NodeID: "confirm_write",
		Prompt: "Approve sheet write?",
	})
	if err != nil || req.Status != hitl.StatusPending {
		t.Fatalf("ask: %+v err=%v", req, err)
	}

	decided, err := ring.Decide("iv-1", "alice", true, "looks good")
	if err != nil || decided.Status != hitl.StatusApproved {
		t.Fatalf("decide: %+v err=%v", decided, err)
	}

	st, ok := ring.WaitResult("iv-1")
	if !ok || st != hitl.StatusApproved {
		t.Fatalf("wait=%s ok=%v", st, ok)
	}
}

func TestTimeout(t *testing.T) {
	ring := hitl.NewRing()
	_, _ = ring.Ask(hitl.Request{
		ID:       "iv-2",
		RunID:    "run-1",
		Deadline: time.Now().UTC().Add(-time.Second),
	})
	if n := ring.TickTimeouts(); n != 1 {
		t.Fatalf("timeouts=%d", n)
	}
	got, _ := ring.Get("iv-2")
	if got.Status != hitl.StatusTimeout {
		t.Fatalf("status=%s", got.Status)
	}
}
