// Package hitl (human-in-the-loop) provides an intervention ring for long
// flows: request → pending → approved|rejected|timeout|canceled.
// No UI required — stubs advance via API so orchestrator tests stay offline.
package hitl

import (
	"fmt"
	"sync"
	"time"
)

// Status of an intervention request.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusTimeout  Status = "timeout"
	StatusCanceled Status = "canceled"
)

// Request is one human gate.
type Request struct {
	ID        string            `json:"id"`
	RunID     string            `json:"run_id"`
	NodeID    string            `json:"node_id"`
	TenantID  string            `json:"tenant_id,omitempty"`
	Prompt    string            `json:"prompt"`
	Status    Status            `json:"status"`
	Actor     string            `json:"actor,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Deadline  time.Time         `json:"deadline,omitempty"`
}

// Ring holds intervention requests (in-process stub).
type Ring struct {
	mu   sync.Mutex
	byID map[string]*Request
	now  func() time.Time
}

// NewRing creates an empty intervention ring.
func NewRing() *Ring {
	return &Ring{byID: map[string]*Request{}, now: time.Now}
}

// Ask opens a pending intervention.
func (r *Ring) Ask(req Request) (*Request, error) {
	if req.ID == "" || req.RunID == "" {
		return nil, fmt.Errorf("hitl: id and run_id required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[req.ID]; ok {
		return nil, fmt.Errorf("hitl: %q exists", req.ID)
	}
	now := r.now().UTC()
	req.Status = StatusPending
	req.CreatedAt = now
	req.UpdatedAt = now
	if req.Meta == nil {
		req.Meta = map[string]string{}
	}
	cp := req
	r.byID[req.ID] = &cp
	return clone(&cp), nil
}

// Decide approves or rejects a pending request.
func (r *Ring) Decide(id, actor string, approve bool, reason string) (*Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("hitl: %q not found", id)
	}
	if req.Status != StatusPending {
		return nil, fmt.Errorf("hitl: %q is %s", id, req.Status)
	}
	if approve {
		req.Status = StatusApproved
	} else {
		req.Status = StatusRejected
	}
	req.Actor = actor
	req.Reason = reason
	req.UpdatedAt = r.now().UTC()
	return clone(req), nil
}

// Cancel marks pending as canceled.
func (r *Ring) Cancel(id, reason string) (*Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("hitl: %q not found", id)
	}
	if req.Status != StatusPending {
		return nil, fmt.Errorf("hitl: %q is %s", id, req.Status)
	}
	req.Status = StatusCanceled
	req.Reason = reason
	req.UpdatedAt = r.now().UTC()
	return clone(req), nil
}

// TickTimeouts marks overdue pending requests as timeout. Returns count.
func (r *Ring) TickTimeouts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UTC()
	n := 0
	for _, req := range r.byID {
		if req.Status == StatusPending && !req.Deadline.IsZero() && now.After(req.Deadline) {
			req.Status = StatusTimeout
			req.UpdatedAt = now
			req.Reason = "deadline exceeded"
			n++
		}
	}
	return n
}

// Get returns a request.
func (r *Ring) Get(id string) (*Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	return clone(req), true
}

// ListByRun returns interventions for a run.
func (r *Ring) ListByRun(runID string) []*Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Request
	for _, req := range r.byID {
		if req.RunID == runID {
			out = append(out, clone(req))
		}
	}
	return out
}

// WaitResult is a non-blocking poll helper for orchestrator stubs.
func (r *Ring) WaitResult(id string) (Status, bool) {
	req, ok := r.Get(id)
	if !ok {
		return "", false
	}
	return req.Status, true
}

func clone(r *Request) *Request {
	cp := *r
	if r.Meta != nil {
		cp.Meta = map[string]string{}
		for k, v := range r.Meta {
			cp.Meta[k] = v
		}
	}
	return &cp
}
