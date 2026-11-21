-- Jobs + flow runs: cross-process idempotency (replaces single-host jobs.json / runs.json).

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    ticker TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    doc_hash TEXT NOT NULL DEFAULT '',
    locator TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    dry_run BOOLEAN NOT NULL DEFAULT false,
    stages JSONB NOT NULL DEFAULT '[]'::jsonb,
    actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    extraction JSONB,
    macro JSONB,
    warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    takeaway JSONB NOT NULL DEFAULT '[]'::jsonb,
    charts JSONB NOT NULL DEFAULT '[]'::jsonb,
    sheet_url TEXT NOT NULL DEFAULT '',
    notified BOOLEAN NOT NULL DEFAULT false,
    cells INT NOT NULL DEFAULT 0,
    tokens INT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Match file-store semantics: only succeeded jobs participate in hash/locator idempotency.
CREATE UNIQUE INDEX IF NOT EXISTS jobs_doc_hash_succeeded_uidx
    ON jobs (doc_hash)
    WHERE status = 'succeeded' AND doc_hash <> '';

CREATE UNIQUE INDEX IF NOT EXISTS jobs_locator_succeeded_uidx
    ON jobs (locator)
    WHERE status = 'succeeded' AND locator <> '';

CREATE INDEX IF NOT EXISTS jobs_ticker_updated_idx
    ON jobs (ticker, updated_at DESC);

CREATE TABLE IF NOT EXISTS flow_runs (
    run_id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    client_request_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    seq INT NOT NULL DEFAULT 0,
    inputs JSONB NOT NULL DEFAULT '{}'::jsonb,
    nodes JSONB NOT NULL DEFAULT '{}'::jsonb,
    output JSONB,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS flow_runs_client_request_id_uidx
    ON flow_runs (client_request_id)
    WHERE client_request_id <> '';

CREATE INDEX IF NOT EXISTS flow_runs_started_at_idx
    ON flow_runs (started_at DESC);

CREATE TABLE IF NOT EXISTS product_events (
    id BIGSERIAL PRIMARY KEY,
    at TIMESTAMPTZ NOT NULL DEFAULT now(),
    name TEXT NOT NULL DEFAULT '',
    run_id TEXT NOT NULL DEFAULT '',
    node_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    attrs JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS product_events_run_id_idx
    ON product_events (run_id, id);
