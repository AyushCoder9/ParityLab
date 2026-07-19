BEGIN;

CREATE SEQUENCE run_number_seq START WITH 1;

ALTER TABLE runs
    ADD COLUMN scenario_name text NOT NULL DEFAULT '',
    ADD COLUMN duration_ms integer NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    ADD COLUMN event_count integer NOT NULL DEFAULT 0 CHECK (event_count >= 0),
    ADD COLUMN finding_count integer NOT NULL DEFAULT 0 CHECK (finding_count >= 0),
    ADD COLUMN recovered boolean NOT NULL DEFAULT false,
    ADD COLUMN snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE run_events
    ADD COLUMN title text NOT NULL DEFAULT '',
    ADD COLUMN detail text NOT NULL DEFAULT '',
    ADD COLUMN is_duplicate boolean NOT NULL DEFAULT false,
    ADD COLUMN snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE reports (
    run_id text PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
    snapshot jsonb NOT NULL,
    generated_at timestamptz NOT NULL,
    content_sha256 bytea NOT NULL CHECK (octet_length(content_sha256) = 32),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    email_hash bytea NOT NULL UNIQUE CHECK (octet_length(email_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE stripe_connections (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    stripe_account_id text NOT NULL,
    sandbox_name text NOT NULL,
    secret_ciphertext bytea NOT NULL,
    secret_key_version smallint NOT NULL DEFAULT 1 CHECK (secret_key_version > 0),
    status text NOT NULL CHECK (status IN ('pending', 'connected', 'invalid', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, stripe_account_id)
);

CREATE TABLE verification_targets (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    base_url text NOT NULL,
    signing_secret_ciphertext bytea NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'connected', 'invalid', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE TABLE scenario_configurations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    scenario_id text NOT NULL,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, scenario_id, version)
);

CREATE TABLE job_leases (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    job_type text NOT NULL,
    status text NOT NULL CHECK (status IN ('queued', 'leased', 'succeeded', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    leased_until timestamptz,
    lease_owner text,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX job_leases_claim_idx ON job_leases (available_at, created_at)
    WHERE status IN ('queued', 'leased');

CREATE TABLE assertions (
    id text NOT NULL,
    run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    name text NOT NULL,
    passed boolean NOT NULL,
    expected text NOT NULL,
    observed text NOT NULL,
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (run_id, id)
);

CREATE TABLE findings (
    id text NOT NULL,
    run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    severity text NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    title text NOT NULL,
    summary text NOT NULL,
    checkpoint text NOT NULL,
    resolved boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    PRIMARY KEY (run_id, id)
);

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id text REFERENCES runs(id) ON DELETE CASCADE,
    kind text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_records (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id uuid REFERENCES projects(id) ON DELETE CASCADE,
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_records_project_time_idx ON audit_records (project_id, created_at DESC);

ALTER TABLE outbox
    ADD COLUMN available_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN locked_at timestamptz,
    ADD COLUMN locked_by text,
    ADD COLUMN last_error_code text;

COMMIT;
