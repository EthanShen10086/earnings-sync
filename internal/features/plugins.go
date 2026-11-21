package features

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Cache is an optional Redis-shaped hot-read / lock placeholder.
type Cache interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// Queue is a lightweight MQ placeholder (async notify / retry).
type Queue interface {
	Enqueue(ctx context.Context, topic string, payload []byte) error
	Len(topic string) int
}

// EventBus is a Kafka-shaped publish placeholder.
type EventBus interface {
	Publish(ctx context.Context, topic string, key string, payload []byte) error
}

// ObjectStore holds filing blobs (dev bucket placeholder).
type ObjectStore interface {
	Put(ctx context.Context, key string, body []byte) error
	Get(ctx context.Context, key string) ([]byte, bool, error)
}

// Auditor records sensitive write actions when FEATURE_AUDIT is on.
type Auditor interface {
	Record(ctx context.Context, action, actor, detail string) error
	Len() int
}

// Searcher is a compliance / fact search placeholder.
type Searcher interface {
	Index(ctx context.Context, id, text string) error
	Find(ctx context.Context, query string) ([]string, error)
}

// --- noops (always safe) ---

type NoopCache struct{}

func (NoopCache) Get(context.Context, string) (string, bool, error) { return "", false, nil }
func (NoopCache) Set(context.Context, string, string, time.Duration) error {
	return nil
}

type NoopQueue struct{}

func (NoopQueue) Enqueue(context.Context, string, []byte) error { return nil }
func (NoopQueue) Len(string) int                                { return 0 }

type NoopBus struct{}

func (NoopBus) Publish(context.Context, string, string, []byte) error { return nil }

type NoopObjects struct{}

func (NoopObjects) Put(context.Context, string, []byte) error { return nil }
func (NoopObjects) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}

type NoopAudit struct{}

func (NoopAudit) Record(context.Context, string, string, string) error { return nil }
func (NoopAudit) Len() int                                             { return 0 }

type NoopSearch struct{}

func (NoopSearch) Index(context.Context, string, string) error { return nil }
func (NoopSearch) Find(context.Context, string) ([]string, error) {
	return nil, nil
}

// --- in-process adapters when flag on + dep placeholder present ---

type MemoryCache struct {
	mu   sync.Mutex
	data map[string]string
}

func (c *MemoryCache) Get(_ context.Context, key string) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		return "", false, nil
	}
	v, ok := c.data[key]
	return v, ok, nil
}

func (c *MemoryCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string]string{}
	}
	c.data[key] = value
	return nil
}

type MemoryQueue struct {
	mu   sync.Mutex
	data map[string][][]byte
}

func (q *MemoryQueue) Enqueue(_ context.Context, topic string, payload []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.data == nil {
		q.data = map[string][][]byte{}
	}
	cp := append([]byte(nil), payload...)
	q.data[topic] = append(q.data[topic], cp)
	return nil
}

func (q *MemoryQueue) Len(topic string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.data[topic])
}

type MemoryBus struct {
	mu     sync.Mutex
	events []struct {
		Topic, Key string
		Payload    []byte
	}
}

func (b *MemoryBus) Publish(_ context.Context, topic, key string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, struct {
		Topic, Key string
		Payload    []byte
	}{Topic: topic, Key: key, Payload: append([]byte(nil), payload...)})
	return nil
}

type MemoryObjects struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (o *MemoryObjects) Put(_ context.Context, key string, body []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.data == nil {
		o.data = map[string][]byte{}
	}
	o.data[key] = append([]byte(nil), body...)
	return nil
}

func (o *MemoryObjects) Get(_ context.Context, key string) ([]byte, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.data[key]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), v...), true, nil
}

type MemoryAudit struct {
	mu      sync.Mutex
	entries []string
}

func (a *MemoryAudit) Record(_ context.Context, action, actor, detail string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, action+"|"+actor+"|"+detail)
	return nil
}

func (a *MemoryAudit) Len() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.entries)
}

type MemorySearch struct {
	mu   sync.Mutex
	docs map[string]string
}

func (s *MemorySearch) Index(_ context.Context, id, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docs == nil {
		s.docs = map[string]string{}
	}
	s.docs[id] = text
	return nil
}

func (s *MemorySearch) Find(_ context.Context, query string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.ToLower(query)
	var out []string
	for id, text := range s.docs {
		if q == "" || strings.Contains(strings.ToLower(text), q) || strings.Contains(strings.ToLower(id), q) {
			out = append(out, id)
		}
	}
	return out, nil
}
