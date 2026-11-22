package kafka

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// Open builds a Bus.
//   - brokers empty + path empty → memory
//   - brokers empty + path set → file JSONL
//   - brokers set → probe TCP; on success return memory wrapped as broker-stub;
//     on failure (non-strict) → file if path set else memory (degraded)
func Open(ctx context.Context, brokers, filePath string, strict bool) (Bus, Status, error) {
	brokers = strings.TrimSpace(brokers)
	filePath = strings.TrimSpace(filePath)
	st := Status{Configured: brokers != ""}

	if brokers == "" {
		if filePath != "" {
			f, err := NewFile(filePath)
			if err != nil {
				return nil, st, err
			}
			st.Backend = "file"
			st.Detail = "KAFKA_BROKERS unset — file event log at " + filePath
			return f, st, nil
		}
		st.Backend = "memory"
		st.Detail = "KAFKA_BROKERS unset — in-process event bus"
		return NewMemory(), st, nil
	}

	live := probeBrokers(ctx, brokers)
	if !live {
		if strict {
			return nil, st, fmt.Errorf("kafka: brokers %s unreachable", brokers)
		}
		st.Degraded = true
		if filePath != "" {
			f, err := NewFile(filePath)
			if err != nil {
				return nil, st, err
			}
			st.Backend = "file"
			st.Detail = fmt.Sprintf("brokers %s unreachable — file bus", brokers)
			return f, st, nil
		}
		st.Backend = "memory"
		st.Detail = fmt.Sprintf("brokers %s unreachable — memory bus", brokers)
		return NewMemory(), st, nil
	}

	// Reachable: still use memory retention as a stub client stand-in.
	st.Backend = "broker-stub"
	st.Detail = fmt.Sprintf("brokers %s reachable; using stub client over memory bus (swap sarama/franz later)", brokers)
	return NewMemory(), st, nil
}

func probeBrokers(ctx context.Context, brokers string) bool {
	for _, part := range strings.Split(brokers, ",") {
		addr := strings.TrimSpace(part)
		if addr == "" {
			continue
		}
		if !strings.Contains(addr, ":") {
			addr = net.JoinHostPort(addr, "9092")
		}
		d := net.Dialer{Timeout: 800 * time.Millisecond}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			continue
		}
		_ = conn.Close()
		return true
	}
	return false
}
