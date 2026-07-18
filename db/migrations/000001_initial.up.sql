BEGIN;

CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    mode text NOT NULL DEFAULT 'sandbox' CHECK (mode = 'sandbox'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE runs (
    id text PRIMARY KEY CHECK (id ~ '^run_[0-9]{6,}$'),
    project_id uuid REFERENCES projects(id) ON DELETE CASCADE,
    scenario_id text NOT NULL,
    fault text NOT NULL CHECK (fault IN ('none', 'duplicate', 'reorder', 'timeout', 'tamper')),
    status text NOT NULL CHECK (status IN ('running', 'passed', 'failed')),
    score smallint NOT NULL CHECK (score BETWEEN 0 AND 100),
    environment text NOT NULL DEFAULT 'sandbox' CHECK (environment = 'sandbox'),
    stripe_object_id text,
    merchant_order_id text,
    started_at timestamptz NOT NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX runs_project_started_idx ON runs (project_id, started_at DESC);

CREATE TABLE run_events (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    sequence integer NOT NULL CHECK (sequence > 0),
    event_type text NOT NULL,
    source text NOT NULL,
    target text NOT NULL,
    status text NOT NULL CHECK (status IN ('healthy', 'diverged', 'recovered', 'blocked')),
    checkpoint text NOT NULL,
    trace_id text NOT NULL,
    latency_ms integer NOT NULL CHECK (latency_ms >= 0),
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL,
    UNIQUE (run_id, sequence)
);

CREATE INDEX run_events_run_time_idx ON run_events (run_id, occurred_at);
CREATE INDEX run_events_trace_idx ON run_events (trace_id);

CREATE TABLE webhook_events (
    stripe_event_id text PRIMARY KEY,
    endpoint_token_hash bytea NOT NULL,
    event_type text NOT NULL,
    livemode boolean NOT NULL CHECK (livemode = false),
    api_version text,
    body_sha256 bytea NOT NULL,
    body_ciphertext bytea,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    processing_error_code text
);

COMMENT ON COLUMN webhook_events.body_ciphertext IS
    'Optional application-encrypted raw body. Never log or expose through product APIs.';

CREATE TABLE idempotency_records (
    scope text NOT NULL,
    idempotency_key_hash bytea NOT NULL,
    request_sha256 bytea NOT NULL,
    response_status integer NOT NULL,
    response_body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (scope, idempotency_key_hash)
);

CREATE INDEX idempotency_expiry_idx ON idempotency_records (expires_at);

CREATE TABLE outbox (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    topic text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0)
);

CREATE INDEX outbox_unpublished_idx ON outbox (created_at) WHERE published_at IS NULL;

COMMIT;
