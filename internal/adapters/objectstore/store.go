// Package objectstore holds filing blobs (local FS stub; S3/CDN later).
package objectstore

import (
	"context"
	"io"
)

// Object is metadata for a stored blob.
type Object struct {
	Key         string
	ContentType string
	Size        int64
	URL         string // optional public/CDN URL
}

// Store persists opaque blobs (earnings PDFs, HTML dumps, etc.).
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, contentType string) (Object, error)
	Get(ctx context.Context, key string) (io.ReadCloser, Object, error)
	Delete(ctx context.Context, key string) error
	Backend() string
}

// CDNConfig is a placeholder for production edge delivery.
type CDNConfig struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"` // e.g. https://cdn.example.com
	Detail  string `json:"detail,omitempty"`
}

// Status describes object store wiring.
type Status struct {
	Backend    string    `json:"backend"`
	Degraded   bool      `json:"degraded"`
	Configured bool      `json:"configured"`
	CDN        CDNConfig `json:"cdn"`
	Detail     string    `json:"detail,omitempty"`
}
