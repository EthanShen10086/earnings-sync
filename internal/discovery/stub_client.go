package discovery

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// stubClient wraps Memory and pretends to talk to Nacos/Consul/Eureka.
// When addr is empty or unreachable it stays on the memory registry (degraded).
// When addr is reachable it still uses memory for Register/Discover (stub SDK)
// but Backend reports the named client so callers can swap later.
type stubClient struct {
	name   string
	addr   string
	live   bool
	memory *Memory
}

func newStub(name, addr string, live bool) *stubClient {
	return &stubClient{name: name, addr: addr, live: live, memory: NewMemory()}
}

func (s *stubClient) Backend() string {
	if s.live {
		return s.name + "-stub"
	}
	return s.name + "-memory"
}

func (s *stubClient) Close() error { return s.memory.Close() }

func (s *stubClient) Ping(ctx context.Context) error {
	if !s.live {
		return s.memory.Ping(ctx)
	}
	return nil
}

func (s *stubClient) Register(ctx context.Context, inst Instance) error {
	if inst.Meta == nil {
		inst.Meta = map[string]string{}
	}
	inst.Meta["discovery_backend"] = s.Backend()
	return s.memory.Register(ctx, inst)
}

func (s *stubClient) Deregister(ctx context.Context, service, id string) error {
	return s.memory.Deregister(ctx, service, id)
}

func (s *stubClient) Discover(ctx context.Context, service string) ([]Instance, error) {
	return s.memory.Discover(ctx, service)
}

func (s *stubClient) ListServices(ctx context.Context) ([]string, error) {
	return s.memory.ListServices(ctx)
}

// probeAddr does a cheap TCP or HTTP check; failures mean "no cluster".
func probeAddr(ctx context.Context, addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
		if err != nil {
			return false
		}
		client := &http.Client{Timeout: 800 * time.Millisecond}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}
	host := addr
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "8848")
	}
	d := net.Dialer{Timeout: 800 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func openNamed(ctx context.Context, name, addr string, strict bool) (Registry, Status, error) {
	st := Status{Backend: name, Configured: strings.TrimSpace(addr) != ""}
	if strings.TrimSpace(addr) == "" {
		st.Backend = name + "-memory"
		st.Degraded = true
		st.Detail = fmt.Sprintf("%s URL unset — using in-process memory registry", name)
		return newStub(name, "", false), st, nil
	}
	live := probeAddr(ctx, addr)
	if !live {
		if strict {
			return nil, st, fmt.Errorf("discovery: %s at %s unreachable", name, addr)
		}
		st.Backend = name + "-memory"
		st.Degraded = true
		st.Detail = fmt.Sprintf("%s at %s unreachable — memory registry", name, addr)
		return newStub(name, addr, false), st, nil
	}
	st.Backend = name + "-stub"
	st.Detail = fmt.Sprintf("%s reachable at %s; using stub client over memory registry (swap SDK later)", name, addr)
	return newStub(name, addr, true), st, nil
}
