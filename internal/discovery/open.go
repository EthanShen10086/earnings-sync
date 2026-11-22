package discovery

import (
	"context"
	"fmt"
)

// Open builds a Registry for backend + optional cluster URL.
// backend: memory | nacos | consul | eureka
// Empty URL always yields a runnable memory registry (never blocks boot).
func Open(ctx context.Context, backend, addr string, strict bool) (Registry, Status, error) {
	name := NormalizeBackend(backend)
	switch name {
	case BackendMemory:
		st := Status{
			Backend:    BackendMemory,
			Degraded:   false,
			Configured: false,
			Detail:     "in-process memory registry",
		}
		return NewMemory(), st, nil
	case BackendNacos, BackendConsul, BackendEureka:
		return openNamed(ctx, name, addr, strict)
	default:
		if strict {
			return nil, Status{}, fmt.Errorf("discovery: unknown backend %q", backend)
		}
		st := Status{
			Backend:  BackendMemory,
			Degraded: true,
			Detail:   fmt.Sprintf("unknown backend %q — falling back to memory", backend),
		}
		return NewMemory(), st, nil
	}
}
