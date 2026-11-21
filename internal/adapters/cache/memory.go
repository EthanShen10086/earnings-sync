package cache

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	value string
	exp   time.Time
}

// Memory is an in-process cache used when Redis is unavailable.
type Memory struct {
	mu   sync.RWMutex
	data map[string]memEntry
}

// NewMemory returns a process-local cache.
func NewMemory() *Memory {
	return &Memory{data: map[string]memEntry{}}
}

func (m *Memory) Get(_ context.Context, key string) (string, bool, error) {
	m.mu.RLock()
	e, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return "", false, nil
	}
	if !e.exp.IsZero() && time.Now().After(e.exp) {
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return "", false, nil
	}
	return e.value, true, nil
}

func (m *Memory) Set(_ context.Context, key, value string, ttl time.Duration) error {
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	m.mu.Lock()
	m.data[key] = memEntry{value: value, exp: exp}
	m.mu.Unlock()
	return nil
}

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) Close() error { return nil }

func (m *Memory) Backend() string { return "memory" }
