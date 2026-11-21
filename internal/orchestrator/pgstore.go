package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PGRunStore persists RunState and ProductEvent in PostgreSQL for cross-process idempotency.
type PGRunStore struct {
	db *sql.DB
}

// NewPGRunStore wraps an already-migrated *sql.DB.
func NewPGRunStore(db *sql.DB) *PGRunStore {
	return &PGRunStore{db: db}
}

var _ RunRegistry = (*PGRunStore)(nil)
var _ EventSink = (*PGRunStore)(nil)

func (s *PGRunStore) Put(state *RunState) {
	if s == nil || state == nil {
		return
	}
	_ = s.upsert(context.Background(), state)
}

func (s *PGRunStore) Get(runID string) (*RunState, bool) {
	if s == nil || runID == "" {
		return nil, false
	}
	st, err := s.queryOne(context.Background(), `SELECT `+runColumns+` FROM flow_runs WHERE run_id = $1`, runID)
	if err != nil {
		return nil, false
	}
	return st, true
}

func (s *PGRunStore) ByClientRequestID(clientID string) (*RunState, bool) {
	if s == nil || clientID == "" {
		return nil, false
	}
	st, err := s.queryOne(context.Background(), `
SELECT `+runColumns+` FROM flow_runs
WHERE client_request_id = $1
ORDER BY started_at DESC
LIMIT 1`, clientID)
	if err != nil {
		return nil, false
	}
	return st, true
}

func (s *PGRunStore) List(limit int) []*RunState {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(context.Background(), `
SELECT `+runColumns+` FROM flow_runs
ORDER BY started_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*RunState
	for rows.Next() {
		st, err := scanRun(rows)
		if err != nil {
			return out
		}
		out = append(out, st)
	}
	return out
}

// Append implements EventSink.
func (s *PGRunStore) Append(ev ProductEvent) error {
	if s == nil {
		return nil
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	attrs, err := json.Marshal(ev.Attrs)
	if err != nil {
		return err
	}
	if ev.Attrs == nil {
		attrs = []byte("{}")
	}
	_, err = s.db.ExecContext(context.Background(), `
INSERT INTO product_events (at, name, run_id, node_id, status, duration_ms, error, attrs)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		ev.At, ev.Name, ev.RunID, ev.NodeID, string(ev.Status), ev.DurationMs, ev.Error, attrs,
	)
	return err
}

// Events returns recent product events (oldest first within the window).
func (s *PGRunStore) Events(limit int) []ProductEvent {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(context.Background(), `
SELECT at, name, run_id, node_id, status, duration_ms, error, attrs
FROM (
  SELECT at, name, run_id, node_id, status, duration_ms, error, attrs
  FROM product_events
  ORDER BY id DESC
  LIMIT $1
) t
ORDER BY at ASC`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []ProductEvent
	for rows.Next() {
		var ev ProductEvent
		var status string
		var attrs []byte
		if err := rows.Scan(&ev.At, &ev.Name, &ev.RunID, &ev.NodeID, &status, &ev.DurationMs, &ev.Error, &attrs); err != nil {
			return out
		}
		ev.Status = Status(status)
		if len(attrs) > 0 && string(attrs) != "null" {
			_ = json.Unmarshal(attrs, &ev.Attrs)
		}
		out = append(out, ev)
	}
	return out
}

const runColumns = `run_id, request_id, trace_id, client_request_id, title, status, seq,
 inputs, nodes, output, error, started_at, ended_at, duration_ms`

func (s *PGRunStore) upsert(ctx context.Context, state *RunState) error {
	inputs, nodes, output, err := marshalRunJSON(state)
	if err != nil {
		return err
	}
	var ended any
	if !state.EndedAt.IsZero() {
		ended = state.EndedAt
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO flow_runs (
  run_id, request_id, trace_id, client_request_id, title, status, seq,
  inputs, nodes, output, error, started_at, ended_at, duration_ms, updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,
  $8,$9,$10,$11,$12,$13,$14, now()
)
ON CONFLICT (run_id) DO UPDATE SET
  request_id = EXCLUDED.request_id,
  trace_id = EXCLUDED.trace_id,
  client_request_id = EXCLUDED.client_request_id,
  title = EXCLUDED.title,
  status = EXCLUDED.status,
  seq = EXCLUDED.seq,
  inputs = EXCLUDED.inputs,
  nodes = EXCLUDED.nodes,
  output = EXCLUDED.output,
  error = EXCLUDED.error,
  ended_at = EXCLUDED.ended_at,
  duration_ms = EXCLUDED.duration_ms,
  updated_at = now()`,
		state.RunID, state.RequestID, state.TraceID, state.ClientRequestID, state.Title, string(state.Status), state.Seq,
		inputs, nodes, output, state.Error, state.StartedAt, ended, state.DurationMs,
	)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func (s *PGRunStore) queryOne(ctx context.Context, q string, args ...any) (*RunState, error) {
	return scanRun(s.db.QueryRowContext(ctx, q, args...))
}

func scanRun(row scannable) (*RunState, error) {
	var st RunState
	var status string
	var inputs, nodes, output []byte
	var ended sql.NullTime
	err := row.Scan(
		&st.RunID, &st.RequestID, &st.TraceID, &st.ClientRequestID, &st.Title, &status, &st.Seq,
		&inputs, &nodes, &output, &st.Error, &st.StartedAt, &ended, &st.DurationMs,
	)
	if err != nil {
		return nil, err
	}
	st.Status = Status(status)
	if ended.Valid {
		st.EndedAt = ended.Time
	}
	if err := unmarshalRunJSON(&st, inputs, nodes, output); err != nil {
		return nil, err
	}
	return cloneRun(&st), nil
}

func marshalRunJSON(state *RunState) (inputs, nodes, output []byte, err error) {
	if inputs, err = json.Marshal(state.Inputs); err != nil {
		return
	}
	if state.Inputs == nil {
		inputs = []byte("{}")
	}
	if nodes, err = json.Marshal(state.Nodes); err != nil {
		return
	}
	if state.Nodes == nil {
		nodes = []byte("{}")
	}
	if state.Output != nil {
		if output, err = json.Marshal(state.Output); err != nil {
			return
		}
	}
	return
}

func unmarshalRunJSON(st *RunState, inputs, nodes, output []byte) error {
	if len(inputs) > 0 {
		if err := json.Unmarshal(inputs, &st.Inputs); err != nil {
			return fmt.Errorf("inputs: %w", err)
		}
	}
	if len(nodes) > 0 {
		if err := json.Unmarshal(nodes, &st.Nodes); err != nil {
			return fmt.Errorf("nodes: %w", err)
		}
	}
	if len(output) > 0 && string(output) != "null" {
		var out any
		if err := json.Unmarshal(output, &out); err != nil {
			return fmt.Errorf("output: %w", err)
		}
		st.Output = out
	}
	return nil
}
