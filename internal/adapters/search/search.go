// Package search is a stub full-text index over facts/audit (file or PG).
package search

import "context"

// Hit is one search result.
type Hit struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
	Snippet string `json:"snippet"`
}

// Document is indexed content.
type Document struct {
	ID    string
	Title string
	Body  string
}

// Index supports basic index + query. Not production search.
type Index interface {
	Upsert(ctx context.Context, doc Document) error
	Query(ctx context.Context, q string, limit int) ([]Hit, error)
	Backend() string
	Close() error
}

// Status describes search wiring.
type Status struct {
	Backend    string `json:"backend"`
	Degraded   bool   `json:"degraded"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
}
