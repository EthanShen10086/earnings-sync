package mq

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Memory is an in-process fan-out queue used when Redis/NATS are unavailable.
type Memory struct {
	mu       sync.Mutex
	seq      atomic.Uint64
	handlers map[string][]func(Message) error
}

// NewMemory returns a process-local queue.
func NewMemory() *Memory {
	return &Memory{handlers: map[string][]func(Message) error{}}
}

func (m *Memory) Backend() string { return "memory" }

func (m *Memory) Close() error { return nil }

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) Publish(_ context.Context, topic string, body []byte) (string, error) {
	id := fmt.Sprintf("mem-%d", m.seq.Add(1))
	msg := Message{ID: id, Topic: topic, Body: append([]byte(nil), body...)}
	m.mu.Lock()
	hs := append([]func(Message) error{}, m.handlers[topic]...)
	m.mu.Unlock()
	for _, h := range hs {
		if err := h(msg); err != nil {
			return id, err
		}
	}
	return id, nil
}

func (m *Memory) Subscribe(ctx context.Context, topic string, handler func(Message) error) error {
	m.mu.Lock()
	m.handlers[topic] = append(m.handlers[topic], handler)
	m.mu.Unlock()
	<-ctx.Done()
	return ctx.Err()
}
