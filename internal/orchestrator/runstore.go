package orchestrator

import (
	"sort"
	"sync"
)

// RunStore is an in-process registry of runs.
// Inspired by ftp3_runtime_hub's client_request_id idempotency, without DB/fleet.
type RunStore struct {
	mu       sync.RWMutex
	byID     map[string]*RunState
	byClient map[string]string // client_request_id -> run_id
	// OnPut is optional; called after each Put with a cloned snapshot (e.g. FileRunStore persist).
	OnPut func(*RunState)
}

// NewRunStore creates an empty store.
func NewRunStore() *RunStore {
	return &RunStore{
		byID:     make(map[string]*RunState),
		byClient: make(map[string]string),
	}
}

// Put upserts a run snapshot (shallow copy of the pointer content is caller's responsibility).
func (s *RunStore) Put(state *RunState) {
	if s == nil || state == nil {
		return
	}
	s.mu.Lock()
	cp := cloneRun(state)
	s.byID[state.RunID] = cp
	if state.ClientRequestID != "" {
		s.byClient[state.ClientRequestID] = state.RunID
	}
	hook := s.OnPut
	s.mu.Unlock()
	if hook != nil {
		hook(cloneRun(state))
	}
}

// Get returns a copy by run id.
func (s *RunStore) Get(runID string) (*RunState, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.byID[runID]
	if !ok {
		return nil, false
	}
	return cloneRun(st), true
}

// ByClientRequestID returns a copy keyed by hub-style client_request_id.
func (s *RunStore) ByClientRequestID(clientID string) (*RunState, bool) {
	if s == nil || clientID == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	runID, ok := s.byClient[clientID]
	if !ok {
		return nil, false
	}
	st, ok := s.byID[runID]
	if !ok {
		return nil, false
	}
	return cloneRun(st), true
}

// List returns recent runs newest-first, capped at limit (default 50).
func (s *RunStore) List(limit int) []*RunState {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*RunState, 0, len(s.byID))
	for _, st := range s.byID {
		out = append(out, cloneRun(st))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func cloneRun(in *RunState) *RunState {
	cp := *in
	cp.Inputs = cloneMap(in.Inputs)
	cp.Nodes = make(map[string]*NodeState, len(in.Nodes))
	for k, v := range in.Nodes {
		n := *v
		if v.Attempts != nil {
			n.Attempts = append([]Attempt(nil), v.Attempts...)
		}
		cp.Nodes[k] = &n
	}
	return &cp
}
