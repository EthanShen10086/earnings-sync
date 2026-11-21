package cache

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

// Redis is a minimal RESP client for GET/SET/PING. No third-party Redis SDK —
// missing or unreachable Redis must not block the minimum path.
type Redis struct {
	addr string
	db   int
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

// DialRedis opens a Redis connection. addr may be host:port or redis:// URL.
func DialRedis(ctx context.Context, raw string) (*Redis, error) {
	addr, db, err := parseRedisAddr(raw)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	r := &Redis{addr: addr, db: db, conn: conn, r: bufio.NewReader(conn)}
	if err := r.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if db > 0 {
		if _, err := r.cmd(ctx, "SELECT", strconv.Itoa(db)); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return r, nil
}

func parseRedisAddr(raw string) (addr string, db int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("empty redis url")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", 0, err
		}
		host := u.Host
		if host == "" {
			return "", 0, fmt.Errorf("redis url missing host")
		}
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), "6379")
		}
		if path := strings.TrimPrefix(u.Path, "/"); path != "" {
			db, _ = strconv.Atoi(path)
		}
		return host, db, nil
	}
	if !strings.Contains(raw, ":") {
		raw = net.JoinHostPort(raw, "6379")
	}
	return raw, 0, nil
}

func (r *Redis) Backend() string { return "redis" }

func (r *Redis) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		return nil
	}
	err := r.conn.Close()
	r.conn = nil
	return err
}

func (r *Redis) Ping(ctx context.Context) error {
	v, err := r.cmd(ctx, "PING")
	if err != nil {
		return err
	}
	if v != "PONG" {
		return fmt.Errorf("redis ping: %q", v)
	}
	return nil
}

func (r *Redis) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := r.cmd(ctx, "GET", key)
	if err != nil {
		if err == errNil {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

func (r *Redis) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if ttl > 0 {
		ms := int(ttl / time.Millisecond)
		if ms < 1 {
			ms = 1
		}
		_, err := r.cmd(ctx, "SET", key, value, "PX", strconv.Itoa(ms))
		return err
	}
	_, err := r.cmd(ctx, "SET", key, value)
	return err
}

var errNil = fmt.Errorf("redis nil")

func (r *Redis) cmd(ctx context.Context, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		return "", fmt.Errorf("redis closed")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = r.conn.SetDeadline(deadline)
	} else {
		_ = r.conn.SetDeadline(time.Now().Add(3 * time.Second))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := r.conn.Write([]byte(b.String())); err != nil {
		return "", err
	}
	return readRESP(r.r)
}

func readRESP(r *bufio.Reader) (string, error) {
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
			return "", errNil
		}
		buf := make([]byte, n+2)
		if _, err := ioReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return "", err
		}
		if n < 0 {
			return "", errNil
		}
		parts := make([]string, 0, n)
		for i := 0; i < n; i++ {
			v, err := readRESP(r)
			if err != nil {
				return "", err
			}
			parts = append(parts, v)
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("unknown redis reply %q", line)
	}
}

func ioReadFull(r *bufio.Reader, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		c, err := r.Read(buf[n:])
		n += c
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// Open prefers Redis when url is set; otherwise Memory. On dial failure and
// !strict, returns Memory with degraded status.
func Open(ctx context.Context, redisURL string, strict bool) (Cache, Status, error) {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return NewMemory(), Status{Backend: "memory", Configured: false, Detail: "REDIS_URL unset"}, nil
	}
	r, err := DialRedis(ctx, redisURL)
	if err != nil {
		if strict {
			return nil, Status{Backend: "redis", Configured: true, Degraded: true, Detail: err.Error()}, err
		}
		return NewMemory(), Status{Backend: "memory", Configured: true, Degraded: true, Detail: err.Error()}, nil
	}
	return r, Status{Backend: "redis", Configured: true}, nil
}
