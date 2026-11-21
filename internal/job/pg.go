package job

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/google/uuid"
)

// PGStore persists jobs in PostgreSQL for cross-process idempotency.
type PGStore struct {
	db *sql.DB
}

// NewPG wraps an existing *sql.DB (migrations must already be applied).
func NewPG(db *sql.DB) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) Create(ticker, hash, locator, source string, dry bool) *model.Job {
	now := time.Now().UTC()
	j := &model.Job{
		ID:        uuid.NewString(),
		Ticker:    ticker,
		Status:    model.StatusRunning,
		DocHash:   hash,
		Locator:   locator,
		Source:    source,
		DryRun:    dry,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.insert(context.Background(), j); err != nil {
		// Keep Create signature compatible with FileStore; surface failure on the job.
		j.Status = model.StatusFailed
		j.Error = err.Error()
	}
	return clone(j)
}

func (s *PGStore) ByHash(hash string) (*model.Job, bool) {
	if hash == "" {
		return nil, false
	}
	j, err := s.queryOne(context.Background(), `
SELECT `+jobColumns+` FROM jobs
WHERE doc_hash = $1 AND status = $2
ORDER BY updated_at DESC
LIMIT 1`, hash, model.StatusSucceeded)
	if err != nil {
		return nil, false
	}
	return j, true
}

func (s *PGStore) ByLocator(locator string) (*model.Job, bool) {
	if locator == "" {
		return nil, false
	}
	j, err := s.queryOne(context.Background(), `
SELECT `+jobColumns+` FROM jobs
WHERE locator = $1 AND status = $2
ORDER BY updated_at DESC
LIMIT 1`, locator, model.StatusSucceeded)
	if err != nil {
		return nil, false
	}
	return j, true
}

func (s *PGStore) Get(id string) (*model.Job, bool) {
	if id == "" {
		return nil, false
	}
	j, err := s.queryOne(context.Background(), `SELECT `+jobColumns+` FROM jobs WHERE id = $1`, id)
	if err != nil {
		return nil, false
	}
	return j, true
}

func (s *PGStore) Save(j *model.Job) error {
	if j == nil {
		return fmt.Errorf("nil job")
	}
	j.UpdatedAt = time.Now().UTC()
	return s.upsert(context.Background(), j)
}

const jobColumns = `id, ticker, kind, status, doc_hash, locator, source, dry_run,
 stages, actions, extraction, macro, warnings, takeaway, charts,
 sheet_url, notified, cells, tokens, error, created_at, updated_at`

func (s *PGStore) insert(ctx context.Context, j *model.Job) error {
	stages, actions, extraction, macro, warnings, takeaway, charts, err := marshalJobJSON(j)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO jobs (
  id, ticker, kind, status, doc_hash, locator, source, dry_run,
  stages, actions, extraction, macro, warnings, takeaway, charts,
  sheet_url, notified, cells, tokens, error, created_at, updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,
  $9,$10,$11,$12,$13,$14,$15,
  $16,$17,$18,$19,$20,$21,$22
)`,
		j.ID, j.Ticker, j.Kind, j.Status, j.DocHash, j.Locator, j.Source, j.DryRun,
		stages, actions, extraction, macro, warnings, takeaway, charts,
		j.SheetURL, j.Notified, j.Cells, j.Tokens, j.Error, j.CreatedAt, j.UpdatedAt,
	)
	return err
}

func (s *PGStore) upsert(ctx context.Context, j *model.Job) error {
	stages, actions, extraction, macro, warnings, takeaway, charts, err := marshalJobJSON(j)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO jobs (
  id, ticker, kind, status, doc_hash, locator, source, dry_run,
  stages, actions, extraction, macro, warnings, takeaway, charts,
  sheet_url, notified, cells, tokens, error, created_at, updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,
  $9,$10,$11,$12,$13,$14,$15,
  $16,$17,$18,$19,$20,$21,$22
)
ON CONFLICT (id) DO UPDATE SET
  ticker = EXCLUDED.ticker,
  kind = EXCLUDED.kind,
  status = EXCLUDED.status,
  doc_hash = EXCLUDED.doc_hash,
  locator = EXCLUDED.locator,
  source = EXCLUDED.source,
  dry_run = EXCLUDED.dry_run,
  stages = EXCLUDED.stages,
  actions = EXCLUDED.actions,
  extraction = EXCLUDED.extraction,
  macro = EXCLUDED.macro,
  warnings = EXCLUDED.warnings,
  takeaway = EXCLUDED.takeaway,
  charts = EXCLUDED.charts,
  sheet_url = EXCLUDED.sheet_url,
  notified = EXCLUDED.notified,
  cells = EXCLUDED.cells,
  tokens = EXCLUDED.tokens,
  error = EXCLUDED.error,
  updated_at = EXCLUDED.updated_at`,
		j.ID, j.Ticker, j.Kind, j.Status, j.DocHash, j.Locator, j.Source, j.DryRun,
		stages, actions, extraction, macro, warnings, takeaway, charts,
		j.SheetURL, j.Notified, j.Cells, j.Tokens, j.Error, j.CreatedAt, j.UpdatedAt,
	)
	return err
}

func (s *PGStore) queryOne(ctx context.Context, q string, args ...any) (*model.Job, error) {
	row := s.db.QueryRowContext(ctx, q, args...)
	var j model.Job
	var stages, actions, extraction, macro, warnings, takeaway, charts []byte
	err := row.Scan(
		&j.ID, &j.Ticker, &j.Kind, &j.Status, &j.DocHash, &j.Locator, &j.Source, &j.DryRun,
		&stages, &actions, &extraction, &macro, &warnings, &takeaway, &charts,
		&j.SheetURL, &j.Notified, &j.Cells, &j.Tokens, &j.Error, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := unmarshalJobJSON(&j, stages, actions, extraction, macro, warnings, takeaway, charts); err != nil {
		return nil, err
	}
	return clone(&j), nil
}

func marshalJobJSON(j *model.Job) (stages, actions, extraction, macro, warnings, takeaway, charts []byte, err error) {
	if stages, err = json.Marshal(j.Stages); err != nil {
		return
	}
	if j.Stages == nil {
		stages = []byte("[]")
	}
	if actions, err = json.Marshal(j.Actions); err != nil {
		return
	}
	if j.Actions == nil {
		actions = []byte("[]")
	}
	if j.Extraction != nil {
		if extraction, err = json.Marshal(j.Extraction); err != nil {
			return
		}
	}
	if j.Macro != nil {
		if macro, err = json.Marshal(j.Macro); err != nil {
			return
		}
	}
	if warnings, err = json.Marshal(j.Warnings); err != nil {
		return
	}
	if j.Warnings == nil {
		warnings = []byte("[]")
	}
	if takeaway, err = json.Marshal(j.Takeaway); err != nil {
		return
	}
	if j.Takeaway == nil {
		takeaway = []byte("[]")
	}
	if charts, err = json.Marshal(j.Charts); err != nil {
		return
	}
	if j.Charts == nil {
		charts = []byte("[]")
	}
	return
}

func unmarshalJobJSON(j *model.Job, stages, actions, extraction, macro, warnings, takeaway, charts []byte) error {
	if len(stages) > 0 {
		if err := json.Unmarshal(stages, &j.Stages); err != nil {
			return err
		}
	}
	if len(actions) > 0 {
		if err := json.Unmarshal(actions, &j.Actions); err != nil {
			return err
		}
	}
	if len(extraction) > 0 && string(extraction) != "null" {
		var ex model.Extraction
		if err := json.Unmarshal(extraction, &ex); err != nil {
			return err
		}
		j.Extraction = &ex
	}
	if len(macro) > 0 && string(macro) != "null" {
		var m model.MacroRelease
		if err := json.Unmarshal(macro, &m); err != nil {
			return err
		}
		j.Macro = &m
	}
	if len(warnings) > 0 {
		if err := json.Unmarshal(warnings, &j.Warnings); err != nil {
			return err
		}
	}
	if len(takeaway) > 0 {
		if err := json.Unmarshal(takeaway, &j.Takeaway); err != nil {
			return err
		}
	}
	if len(charts) > 0 {
		if err := json.Unmarshal(charts, &j.Charts); err != nil {
			return err
		}
	}
	return nil
}
