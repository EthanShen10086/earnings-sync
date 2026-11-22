// Package discovery is a service-registry skeleton (Nacos / Consul / Eureka).
// Without a cluster URL it uses an in-process memory registry; with a URL it
// still runs the same Register/Discover logic and marks the backend as a stub
// client ready to swap for a real SDK.
package discovery

import (
	"context"
	"strings"
)

// Instance is one registered service endpoint.
type Instance struct {
	ID      string            `json:"id"`
	Service string            `json:"service"`
	Host    string            `json:"host"`
	Port    int               `json:"port"`
	Meta    map[string]string `json:"meta,omitempty"`
	Healthy bool              `json:"healthy"`
}

// Registry registers and discovers service instances.
type Registry interface {
	Register(ctx context.Context, inst Instance) error
	Deregister(ctx context.Context, service, id string) error
	Discover(ctx context.Context, service string) ([]Instance, error)
	ListServices(ctx context.Context) ([]string, error)
	Ping(ctx context.Context) error
	Close() error
	Backend() string
}

// Status describes how discovery was wired.
type Status struct {
	Backend    string `json:"backend"`
	Degraded   bool   `json:"degraded"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
}

// Backend names accepted by Open.
const (
	BackendMemory = "memory"
	BackendNacos  = "nacos"
	BackendConsul = "consul"
	BackendEureka = "eureka"
)

// NormalizeBackend maps user input to a known backend name.
func NormalizeBackend(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "mem", "memory", "inmemory", "local":
		return BackendMemory
	case "nacos":
		return BackendNacos
	case "consul":
		return BackendConsul
	case "eureka":
		return BackendEureka
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}
