-- earnings-sync dual-write schema
-- Idempotency: UNIQUE(idempotency_key) so re-ingest of the same fact is a no-op upsert.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS earnings_facts (
    id BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    spreadsheet_id TEXT NOT NULL DEFAULT '',
    ticker TEXT NOT NULL,
    fiscal_year INT NOT NULL,
    fiscal_quarter INT NOT NULL,
    statement TEXT NOT NULL DEFAULT '',
    line_item TEXT NOT NULL,
    value DOUBLE PRECISION,
    text_value TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT earnings_facts_idempotency UNIQUE (idempotency_key)
);

CREATE INDEX IF NOT EXISTS earnings_facts_ticker_period_idx
    ON earnings_facts (ticker, fiscal_year, fiscal_quarter);

CREATE TABLE IF NOT EXISTS earnings_charts (
    id BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    spreadsheet_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    chart_type TEXT NOT NULL DEFAULT '',
    sheet_name TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT earnings_charts_idempotency UNIQUE (idempotency_key)
);

CREATE TABLE IF NOT EXISTS macro_points (
    id BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    spreadsheet_id TEXT NOT NULL DEFAULT '',
    series_id TEXT NOT NULL DEFAULT '',
    panel_id TEXT NOT NULL DEFAULT '',
    sheet_name TEXT NOT NULL DEFAULT '',
    period TEXT NOT NULL DEFAULT '',
    basis TEXT NOT NULL DEFAULT '',
    indicator TEXT NOT NULL DEFAULT '',
    window_name TEXT NOT NULL DEFAULT '',
    year_value INT,
    value DOUBLE PRECISION,
    layout TEXT NOT NULL DEFAULT 'rate',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT macro_points_idempotency UNIQUE (idempotency_key)
);

CREATE TABLE IF NOT EXISTS write_attempts (
    id BIGSERIAL PRIMARY KEY,
    target TEXT NOT NULL,
    op TEXT NOT NULL,
    spreadsheet_id TEXT NOT NULL DEFAULT '',
    ok BOOLEAN NOT NULL,
    cells INT NOT NULL DEFAULT 0,
    err TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
