// Package mq is a lightweight message queue (Redis Streams / NATS / memory).
package mq

import "context"

// Message is one queued payload.
type Message struct {
	ID    string
	Topic string
	Body  []byte
}

// Queue publishes and consumes async work. Missing backends degrade to Memory.
type Queue interface {
	Publish(ctx context.Context, topic string, body []byte) (id string, err error)
	// Subscribe registers a handler for topic. Implementations may run inline
	// (memory) or poll (streams). Cancel ctx to stop.
	Subscribe(ctx context.Context, topic string, handler func(Message) error) error
	Ping(ctx context.Context) error
	Close() error
	Backend() string
}

// Status describes how the queue was wired.
type Status struct {
	Backend    string `json:"backend"`
	Degraded   bool   `json:"degraded"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
}
