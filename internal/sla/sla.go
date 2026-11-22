// Package sla provides SLA check and backup-drill hooks. Real restores need a
// cluster; here the state machine runs locally so runbooks and CI can tick
// through scheduled → running → passed|failed without external deps.
package sla

import (
	"fmt"
	"sync"
	"time"
)

// DrillPhase for backup / restore drills.
type DrillPhase string

const (
	DrillScheduled DrillPhase = "scheduled"
	DrillRunning   DrillPhase = "running"
	DrillPassed    DrillPhase = "passed"
	DrillFailed    DrillPhase = "failed"
	DrillSkipped   DrillPhase = "skipped"
)

// Drill is one backup/restore exercise.
type Drill struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"` // backup | restore | failover
	TenantID  string        `json:"tenant_id,omitempty"`
	Phase     DrillPhase    `json:"phase"`
	TargetRPO time.Duration `json:"target_rpo_ns"`
	TargetRTO time.Duration `json:"target_rto_ns"`
	Observed  time.Duration `json:"observed_ns"`
	Notes     string        `json:"notes,omitempty"`
	Hooks     []string      `json:"hooks,omitempty"` // invoked hook names
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Check is a lightweight SLO probe result.
type Check struct {
	Name    string        `json:"name"`
	OK      bool          `json:"ok"`
	Latency time.Duration `json:"latency_ns"`
	Detail  string        `json:"detail,omitempty"`
	At      time.Time     `json:"at"`
}

// HookFunc is invoked during drill phases (e.g. snapshot, verify).
type HookFunc func(d *Drill) error

// Board holds drills + hooks (stub control plane).
type Board struct {
	mu     sync.Mutex
	drills map[string]*Drill
	hooks  map[string]HookFunc
	checks []Check
	now    func() time.Time
}

// NewBoard creates an SLA board with built-in noop hooks.
func NewBoard() *Board {
	b := &Board{
		drills: map[string]*Drill{},
		hooks:  map[string]HookFunc{},
		now:    time.Now,
	}
	b.RegisterHook("preflight", func(d *Drill) error {
		d.Notes = trimJoin(d.Notes, "preflight ok")
		return nil
	})
	b.RegisterHook("snapshot", func(d *Drill) error {
		d.Notes = trimJoin(d.Notes, "snapshot stub")
		return nil
	})
	b.RegisterHook("verify", func(d *Drill) error {
		d.Notes = trimJoin(d.Notes, "verify stub")
		return nil
	})
	return b
}

// RegisterHook adds/overrides a named hook.
func (b *Board) RegisterHook(name string, fn HookFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hooks[name] = fn
}

// ScheduleDrill queues a drill.
func (b *Board) ScheduleDrill(d Drill) (*Drill, error) {
	if d.ID == "" || d.Kind == "" {
		return nil, fmt.Errorf("sla: id and kind required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.drills[d.ID]; ok {
		return nil, fmt.Errorf("sla: drill %q exists", d.ID)
	}
	now := b.now().UTC()
	d.Phase = DrillScheduled
	d.CreatedAt = now
	d.UpdatedAt = now
	if d.TargetRPO == 0 {
		d.TargetRPO = time.Hour
	}
	if d.TargetRTO == 0 {
		d.TargetRTO = 30 * time.Minute
	}
	cp := d
	b.drills[d.ID] = &cp
	return cloneDrill(&cp), nil
}

// RunDrill executes hooks: preflight → snapshot → verify, then pass/fail by RTO.
func (b *Board) RunDrill(id string, observed time.Duration) (*Drill, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.drills[id]
	if !ok {
		return nil, fmt.Errorf("sla: drill %q not found", id)
	}
	if d.Phase != DrillScheduled && d.Phase != DrillFailed {
		return nil, fmt.Errorf("sla: drill %q is %s", id, d.Phase)
	}
	d.Phase = DrillRunning
	d.UpdatedAt = b.now().UTC()
	d.Hooks = d.Hooks[:0]
	for _, name := range []string{"preflight", "snapshot", "verify"} {
		fn := b.hooks[name]
		if fn == nil {
			continue
		}
		d.Hooks = append(d.Hooks, name)
		if err := fn(d); err != nil {
			d.Phase = DrillFailed
			d.Notes = trimJoin(d.Notes, err.Error())
			d.UpdatedAt = b.now().UTC()
			return cloneDrill(d), err
		}
	}
	d.Observed = observed
	if observed > d.TargetRTO {
		d.Phase = DrillFailed
		d.Notes = trimJoin(d.Notes, "RTO exceeded")
	} else {
		d.Phase = DrillPassed
	}
	d.UpdatedAt = b.now().UTC()
	return cloneDrill(d), nil
}

// RecordCheck appends an SLA probe result.
func (b *Board) RecordCheck(c Check) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c.At.IsZero() {
		c.At = b.now().UTC()
	}
	b.checks = append(b.checks, c)
}

// RecentChecks returns the last n checks (newest last).
func (b *Board) RecentChecks(n int) []Check {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n <= 0 || n > len(b.checks) {
		n = len(b.checks)
	}
	out := make([]Check, n)
	copy(out, b.checks[len(b.checks)-n:])
	return out
}

// GetDrill returns a drill.
func (b *Board) GetDrill(id string) (*Drill, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.drills[id]
	if !ok {
		return nil, false
	}
	return cloneDrill(d), true
}

// ListDrills returns all drills.
func (b *Board) ListDrills() []*Drill {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*Drill, 0, len(b.drills))
	for _, d := range b.drills {
		out = append(out, cloneDrill(d))
	}
	return out
}

func cloneDrill(d *Drill) *Drill {
	cp := *d
	if d.Hooks != nil {
		cp.Hooks = append([]string{}, d.Hooks...)
	}
	return &cp
}

func trimJoin(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "; " + b
}
