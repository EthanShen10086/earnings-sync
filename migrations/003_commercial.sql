-- Multi-tenant / lakehouse analytic mirror (optional; ORM Memory works without PG).

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    plan TEXT NOT NULL DEFAULT 'free',
    quota_rps INT NOT NULL DEFAULT 10,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO tenants (id, name, plan, quota_rps, active)
VALUES ('default', 'Default', 'free', 10, TRUE)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS lakehouse_facts (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'default',
    ticker TEXT NOT NULL,
    period TEXT NOT NULL DEFAULT '',
    line_item TEXT NOT NULL,
    value DOUBLE PRECISION,
    event_time TIMESTAMPTZ NOT NULL,
    partition TEXT NOT NULL,
    year_month TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS lakehouse_facts_partition_idx
    ON lakehouse_facts (partition);

CREATE INDEX IF NOT EXISTS lakehouse_facts_tenant_ticker_idx
    ON lakehouse_facts (tenant_id, ticker, period);
