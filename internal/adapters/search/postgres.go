package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresIndex is a stub using ILIKE over a documents table.
type PostgresIndex struct {
	pool *pgxpool.Pool
}

const ensureSearchSQL = `
CREATE TABLE IF NOT EXISTS search_documents (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// DialPostgres opens PG search stub.
func DialPostgres(ctx context.Context, databaseURL string) (*PostgresIndex, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := pool.Exec(ctx, ensureSearchSQL); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresIndex{pool: pool}, nil
}

func (p *PostgresIndex) Backend() string { return "postgres" }

func (p *PostgresIndex) Close() error {
	p.pool.Close()
	return nil
}

func (p *PostgresIndex) Upsert(ctx context.Context, doc Document) error {
	_, err := p.pool.Exec(ctx, `
INSERT INTO search_documents (id, title, body, updated_at)
VALUES ($1,$2,$3,now())
ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, body=EXCLUDED.body, updated_at=now()`,
		doc.ID, doc.Title, doc.Body)
	return err
}

func (p *PostgresIndex) Query(ctx context.Context, q string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 20
	}
	q = strings.TrimSpace(q)
	rows, err := p.pool.Query(ctx, `
SELECT id, title, body FROM search_documents
WHERE ($1 = '' OR title ILIKE '%'||$1||'%' OR body ILIKE '%'||$1||'%')
ORDER BY updated_at DESC LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var id, title, body string
		if err := rows.Scan(&id, &title, &body); err != nil {
			return nil, err
		}
		snip := title
		if snip == "" {
			snip = body
		}
		if len(snip) > 120 {
			snip = snip[:120]
		}
		hits = append(hits, Hit{ID: id, Score: 1, Snippet: snip})
	}
	return hits, rows.Err()
}

// Open prefers Postgres when reachable; otherwise file index.
func Open(ctx context.Context, dir, databaseURL string, strict bool) (Index, Status, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL != "" {
		pg, err := DialPostgres(ctx, databaseURL)
		if err == nil {
			return pg, Status{Backend: "postgres", Configured: true}, nil
		}
		if strict {
			return nil, Status{Backend: "postgres", Configured: true, Degraded: true, Detail: err.Error()}, err
		}
		f, ferr := NewFileIndex(dir)
		if ferr != nil {
			return nil, Status{Backend: "file", Configured: true, Degraded: true, Detail: fmt.Sprintf("pg: %v; file: %v", err, ferr)}, ferr
		}
		return f, Status{Backend: "file", Configured: true, Degraded: true, Detail: err.Error()}, nil
	}
	f, err := NewFileIndex(dir)
	if err != nil {
		return nil, Status{Backend: "file", Degraded: true, Detail: err.Error()}, err
	}
	return f, Status{Backend: "file", Configured: strings.TrimSpace(dir) != ""}, nil
}
