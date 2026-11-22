package kafka

import (
	"context"
	"sync"
)

// Memory retains events in-process and fans out to subscribers.
type Memory struct {
	mu       sync.Mutex
	events   []Event
	handlers map[string][]func(Event) error
	offset   int64
}

// NewMemory returns an empty in-process bus.
func NewMemory() *Memory {
	return &Memory{handlers: map[string][]func(Event) error{}}
}

func (m *Memory) Backend() string { return "memory" }

func (m *Memory) Close() error { return nil }

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *Memory) Snapshot() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Event, len(m.events))
	copy(out, m.events)
	for i := range out {
		out[i].Payload = append([]byte(nil), out[i].Payload...)
	}
	return out
}

func (m *Memory) Publish(_ context.Context, topic, key string, payload []byte) error {
	m.mu.Lock()
	m.offset++
	ev := Event{
		Topic:   topic,
		Key:     key,
		Payload: append([]byte(nil), payload...),
		Offset:  m.offset,
	}
	m.events = append(m.events, ev)
	hs := append([]func(Event) error{}, m.handlers[topic]...)
	m.mu.Unlock()
	for _, h := range hs {
		if err := h(ev); err != nil {
			return err
		}
	}
	return nil
}

func (m *Memory) Subscribe(ctx context.Context, topic string, handler func(Event) error) error {
	m.mu.Lock()
	// Replay retained events for this topic.
	var replay []Event
	for _, e := range m.events {
		if e.Topic == topic {
			replay = append(replay, e)
		}
	}
	m.handlers[topic] = append(m.handlers[topic], handler)
	m.mu.Unlock()
	for _, e := range replay {
		if err := handler(e); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}
