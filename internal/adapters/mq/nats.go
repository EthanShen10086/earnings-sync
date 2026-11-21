package mq

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// NATS is a TCP connectivity stub. Full protocol client is deferred; dial
// success proves local NATS is up, publish/subscribe stay in-memory until
// a real nats.go dependency is justified.
type NATS struct {
	addr   string
	conn   net.Conn
	memory *Memory
	mu     sync.Mutex
}

// DialNATS checks that a NATS server accepts TCP connections.
func DialNATS(ctx context.Context, raw string) (*NATS, error) {
	addr, err := parseAddr(raw, "4222")
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// Read INFO line if present; ignore protocol details for this stub.
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 512)
	_, _ = conn.Read(buf)
	_ = conn.SetReadDeadline(time.Time{})
	return &NATS{addr: addr, conn: conn, memory: NewMemory()}, nil
}

func (n *NATS) Backend() string { return "nats-stub" }

func (n *NATS) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.conn == nil {
		return nil
	}
	err := n.conn.Close()
	n.conn = nil
	return err
}

func (n *NATS) Ping(context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.conn == nil {
		return fmt.Errorf("nats closed")
	}
	return nil
}

func (n *NATS) Publish(ctx context.Context, topic string, body []byte) (string, error) {
	return n.memory.Publish(ctx, topic, body)
}

func (n *NATS) Subscribe(ctx context.Context, topic string, handler func(Message) error) error {
	return n.memory.Subscribe(ctx, topic, handler)
}

// Open picks Redis Streams, then NATS, then Memory. Dial failures degrade
// unless strict is set.
func Open(ctx context.Context, redisURL, natsURL string, strict bool) (Queue, Status, error) {
	redisURL = strings.TrimSpace(redisURL)
	natsURL = strings.TrimSpace(natsURL)

	if redisURL != "" {
		q, err := DialRedisStreams(ctx, redisURL)
		if err == nil {
			return q, Status{Backend: "redis-streams", Configured: true}, nil
		}
		if strict {
			return nil, Status{Backend: "redis-streams", Configured: true, Degraded: true, Detail: err.Error()}, err
		}
		// Fall through to NATS / memory with degrade note.
		if natsURL == "" {
			return NewMemory(), Status{Backend: "memory", Configured: true, Degraded: true, Detail: err.Error()}, nil
		}
	}

	if natsURL != "" {
		q, err := DialNATS(ctx, natsURL)
		if err == nil {
			return q, Status{Backend: "nats-stub", Configured: true}, nil
		}
		if strict {
			return nil, Status{Backend: "nats-stub", Configured: true, Degraded: true, Detail: err.Error()}, err
		}
		return NewMemory(), Status{Backend: "memory", Configured: true, Degraded: true, Detail: err.Error()}, nil
	}

	return NewMemory(), Status{Backend: "memory", Configured: false, Detail: "REDIS_URL/NATS_URL unset"}, nil
}
