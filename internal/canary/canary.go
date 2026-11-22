// Package canary implements flow gray-release (canary) as a runnable state
// machine. Without a real cluster it still advances states and records traffic
// split decisions so CI / local demos stay deterministic.
package canary

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

// Phase is the canary lifecycle.
type Phase string

const (
	PhaseDraft      Phase = "draft"
	PhaseCanary     Phase = "canary"
	PhasePromoted   Phase = "promoted"
	PhaseRolledBack Phase = "rolled_back"
)

// Release tracks one flow version rollout.
type Release struct {
	ID           string    `json:"id"`
	FlowID       string    `json:"flow_id"`
	StableVer    string    `json:"stable_version"`
	CanaryVer    string    `json:"canary_version"`
	Percent      int       `json:"percent"` // 0–100 traffic to canary
	Phase        Phase     `json:"phase"`
	TenantID     string    `json:"tenant_id,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	Decisions    int       `json:"decisions"`
	CanaryHits   int       `json:"canary_hits"`
	StableHits   int       `json:"stable_hits"`
	LastDecision string    `json:"last_decision,omitempty"` // stable | canary
}

// Controller holds releases in memory (cluster stub).
type Controller struct {
	mu       sync.Mutex
	releases map[string]*Release
	now      func() time.Time
}

// NewController creates an empty controller.
func NewController() *Controller {
	return &Controller{
		releases: map[string]*Release{},
		now:      time.Now,
	}
}

// Start creates a draft release.
func (c *Controller) Start(id, flowID, stable, canary, tenantID string) (*Release, error) {
	if id == "" || flowID == "" {
		return nil, fmt.Errorf("canary: id and flow_id required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.releases[id]; ok {
		return nil, fmt.Errorf("canary: release %q exists", id)
	}
	r := &Release{
		ID:        id,
		FlowID:    flowID,
		StableVer: stable,
		CanaryVer: canary,
		Percent:   0,
		Phase:     PhaseDraft,
		TenantID:  tenantID,
		UpdatedAt: c.now().UTC(),
	}
	c.releases[id] = r
	return clone(r), nil
}

// SetCanary moves draft → canary with percent traffic.
func (c *Controller) SetCanary(id string, percent int) (*Release, error) {
	if percent < 0 || percent > 100 {
		return nil, fmt.Errorf("canary: percent out of range")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.releases[id]
	if !ok {
		return nil, fmt.Errorf("canary: %q not found", id)
	}
	if r.Phase == PhasePromoted || r.Phase == PhaseRolledBack {
		return nil, fmt.Errorf("canary: release %q is %s", id, r.Phase)
	}
	r.Percent = percent
	r.Phase = PhaseCanary
	r.UpdatedAt = c.now().UTC()
	return clone(r), nil
}

// Promote marks canary as the new stable (percent forced to 100 then done).
func (c *Controller) Promote(id string) (*Release, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.releases[id]
	if !ok {
		return nil, fmt.Errorf("canary: %q not found", id)
	}
	if r.Phase != PhaseCanary && r.Phase != PhaseDraft {
		return nil, fmt.Errorf("canary: cannot promote from %s", r.Phase)
	}
	r.Phase = PhasePromoted
	r.Percent = 100
	r.StableVer = r.CanaryVer
	r.UpdatedAt = c.now().UTC()
	return clone(r), nil
}

// Rollback returns traffic to stable and freezes the release.
func (c *Controller) Rollback(id string) (*Release, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.releases[id]
	if !ok {
		return nil, fmt.Errorf("canary: %q not found", id)
	}
	if r.Phase == PhasePromoted {
		return nil, fmt.Errorf("canary: already promoted; start a new release")
	}
	r.Phase = PhaseRolledBack
	r.Percent = 0
	r.UpdatedAt = c.now().UTC()
	return clone(r), nil
}

// PickVersion chooses stable or canary for a routing key (user/run id).
func (c *Controller) PickVersion(id, routingKey string) (version string, bucket string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.releases[id]
	if !ok {
		return "", "", fmt.Errorf("canary: %q not found", id)
	}
	r.Decisions++
	switch r.Phase {
	case PhaseDraft, PhaseRolledBack:
		r.StableHits++
		r.LastDecision = "stable"
		return r.StableVer, "stable", nil
	case PhasePromoted:
		r.CanaryHits++
		r.LastDecision = "canary"
		return r.CanaryVer, "canary", nil
	case PhaseCanary:
		if inCanaryBucket(routingKey, r.Percent) {
			r.CanaryHits++
			r.LastDecision = "canary"
			return r.CanaryVer, "canary", nil
		}
		r.StableHits++
		r.LastDecision = "stable"
		return r.StableVer, "stable", nil
	default:
		return r.StableVer, "stable", nil
	}
}

// Get returns a release snapshot.
func (c *Controller) Get(id string) (*Release, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.releases[id]
	if !ok {
		return nil, false
	}
	return clone(r), true
}

// List returns all releases.
func (c *Controller) List() []*Release {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*Release, 0, len(c.releases))
	for _, r := range c.releases {
		out = append(out, clone(r))
	}
	return out
}

func inCanaryBucket(key string, percent int) bool {
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32()%100) < percent
}

func clone(r *Release) *Release {
	cp := *r
	return &cp
}
