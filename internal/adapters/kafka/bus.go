// Package kafka is a Kafka-shaped event bus (memory / file / broker stub).
// Without brokers the bus retains events in memory or append-only file so
// Publish/Subscribe stay runnable; with brokers present a dialable stub is used
// until a real client is wired.
package kafka

import "context"

// Event is one published record.
type Event struct {
	Topic   string `json:"topic"`
	Key     string `json:"key,omitempty"`
	Payload []byte `json:"payload"`
	Offset  int64  `json:"offset"`
}

// Bus publishes and optionally retains Kafka-shaped events.
type Bus interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
	// Subscribe delivers retained + future events for topic until ctx done.
	Subscribe(ctx context.Context, topic string, handler func(Event) error) error
	// Snapshot returns a copy of retained events (memory/file backends).
	Snapshot() []Event
	Len() int
	Ping(ctx context.Context) error
	Close() error
	Backend() string
}

// Status describes how the bus was wired.
type Status struct {
	Backend    string `json:"backend"`
	Degraded   bool   `json:"degraded"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
}
