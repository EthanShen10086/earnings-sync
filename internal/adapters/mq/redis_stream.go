package mq

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RedisStreams publishes via XADD. Subscribe is a placeholder poll loop so
// local Redis can be exercised without pulling a full client SDK.
type RedisStreams struct {
	addr string
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

// DialRedisStreams connects to Redis for stream publish.
func DialRedisStreams(ctx context.Context, raw string) (*RedisStreams, error) {
	addr, err := parseAddr(raw, "6379")
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	q := &RedisStreams{addr: addr, conn: conn, r: bufio.NewReader(conn)}
	if err := q.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return q, nil
}

func (q *RedisStreams) Backend() string { return "redis-streams" }

func (q *RedisStreams) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.conn == nil {
		return nil
	}
	err := q.conn.Close()
	q.conn = nil
	return err
}

func (q *RedisStreams) Ping(ctx context.Context) error {
	v, err := q.cmd(ctx, "PING")
	if err != nil {
		return err
	}
	if v != "PONG" {
		return fmt.Errorf("redis ping: %q", v)
	}
	return nil
}

func (q *RedisStreams) Publish(ctx context.Context, topic string, body []byte) (string, error) {
	stream := "earnings:" + topic
	return q.cmd(ctx, "XADD", stream, "*", "body", string(body))
}

// Subscribe is intentionally a no-op wait: production would XREADGROUP.
// Keeping it inert avoids spinning CI when Redis is only a placeholder.
func (q *RedisStreams) Subscribe(ctx context.Context, _ string, _ func(Message) error) error {
	<-ctx.Done()
	return ctx.Err()
}

func (q *RedisStreams) cmd(ctx context.Context, args ...string) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.conn == nil {
		return "", fmt.Errorf("redis closed")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = q.conn.SetDeadline(deadline)
	} else {
		_ = q.conn.SetDeadline(time.Now().Add(3 * time.Second))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := q.conn.Write([]byte(b.String())); err != nil {
		return "", err
	}
	return readSimpleRESP(q.r)
}

func parseAddr(raw, defaultPort string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty address")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		host := u.Host
		if host == "" {
			return "", fmt.Errorf("url missing host")
		}
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), defaultPort)
		}
		return host, nil
	}
	if !strings.Contains(raw, ":") {
		raw = net.JoinHostPort(raw, defaultPort)
	}
	return raw, nil
}

func readSimpleRESP(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" {
		return "", fmt.Errorf("empty redis response")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return "", fmt.Errorf("redis error: %s", line[1:])
	case ':':
		return line[1:], nil
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return "", err
		}
		if n < 0 {
			return "", nil
		}
		buf := make([]byte, n+2)
		got := 0
		for got < len(buf) {
			c, err := r.Read(buf[got:])
			got += c
			if err != nil {
				return "", err
			}
		}
		return string(buf[:n]), nil
	case '*':
		// XADD returns a bulk string id; arrays are rare for our commands.
		return line, nil
	default:
		return "", fmt.Errorf("unknown redis reply %q", line)
	}
}
