package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresLogger is a stub that writes to an audit_events table when PG is up.
// Schema is created lazily; missing PG falls back to FileLogger via Open.
type PostgresLogger struct {
	pool *pgxpool.Pool
}

const ensureSQL = `
CREATE TABLE IF NOT EXISTS audit_events (
  id BIGSERIAL PRIMARY KEY,
  at TIMESTAMPTZ NOT NULL DEFAULT now(),
  actor TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL DEFAULT '',
  target TEXT NOT NULL DEFAULT '',
  meta JSONB NOT NULL DEFAULT '{}'::jsonb
);
`

// DialPostgres opens a pool and ensures the stub table exists.
func DialPostgres(ctx context.Context, databaseURL string) (*PostgresLogger, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := pool.Exec(ctx, ensureSQL); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresLogger{pool: pool}, nil
}

func (p *PostgresLogger) Backend() string { return "postgres" }

func (p *PostgresLogger) Close() error {
	p.pool.Close()
	return nil
}

func (p *PostgresLogger) Append(ctx context.Context, e Event) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	meta, err := json.Marshal(e.Meta)
	if err != nil {
		meta = []byte("{}")
	}
	_, err = p.pool.Exec(ctx,
		`INSERT INTO audit_events (at, actor, action, target, meta) VALUES ($1,$2,$3,$4,$5)`,
		e.At, e.Actor, e.Action, e.Target, meta,
	)
	return err
}

func (p *PostgresLogger) List(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx,
		`SELECT at, actor, action, target, meta FROM audit_events ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var meta []byte
		if err := rows.Scan(&e.At, &e.Actor, &e.Action, &e.Target, &meta); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meta, &e.Meta)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Open prefers Postgres when databaseURL is set and reachable; otherwise file.
func Open(ctx context.Context, path, databaseURL string, strict bool) (Logger, Status, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL != "" {
		pg, err := DialPostgres(ctx, databaseURL)
		if err == nil {
			return pg, Status{Backend: "postgres", Configured: true}, nil
		}
		if strict {
			return nil, Status{Backend: "postgres", Configured: true, Degraded: true, Detail: err.Error()}, err
		}
		f, ferr := NewFileLogger(path)
		if ferr != nil {
			return nil, Status{Backend: "file", Configured: true, Degraded: true, Detail: fmt.Sprintf("pg: %v; file: %v", err, ferr)}, ferr
		}
		return f, Status{Backend: "file", Configured: true, Degraded: true, Detail: err.Error()}, nil
	}
	f, err := NewFileLogger(path)
	if err != nil {
		return nil, Status{Backend: "file", Degraded: true, Detail: err.Error()}, err
	}
	return f, Status{Backend: "file", Configured: strings.TrimSpace(path) != ""}, nil
}
