package discovery

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Memory is an in-process service registry used when no cluster is configured.
type Memory struct {
	mu   sync.RWMutex
	byID map[string]Instance // service+"/"+id
}

// NewMemory returns an empty registry.
func NewMemory() *Memory {
	return &Memory{byID: map[string]Instance{}}
}

func (m *Memory) Backend() string { return BackendMemory }

func (m *Memory) Close() error { return nil }

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) Register(_ context.Context, inst Instance) error {
	if inst.Service == "" {
		return fmt.Errorf("discovery: service name required")
	}
	if inst.ID == "" {
		inst.ID = fmt.Sprintf("%s-%s-%d", inst.Service, inst.Host, inst.Port)
	}
	if inst.Host == "" {
		inst.Host = "127.0.0.1"
	}
	inst.Healthy = true
	key := inst.Service + "/" + inst.ID
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := inst
	if inst.Meta != nil {
		cp.Meta = map[string]string{}
		for k, v := range inst.Meta {
			cp.Meta[k] = v
		}
	}
	m.byID[key] = cp
	return nil
}

func (m *Memory) Deregister(_ context.Context, service, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID, service+"/"+id)
	return nil
}

func (m *Memory) Discover(_ context.Context, service string) ([]Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Instance
	prefix := service + "/"
	for k, v := range m.byID {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host == out[j].Host {
			return out[i].Port < out[j].Port
		}
		return out[i].Host < out[j].Host
	})
	return out, nil
}

func (m *Memory) ListServices(_ context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := map[string]struct{}{}
	for _, v := range m.byID {
		seen[v.Service] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

// Len returns registered instance count (tests / status).
func (m *Memory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byID)
}
