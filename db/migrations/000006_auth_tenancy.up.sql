BEGIN;

ALTER TABLE users
    ADD COLUMN email_ciphertext bytea,
    ADD COLUMN password_hash text,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE projects
    ADD COLUMN retention_days integer NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 3650),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at) WHERE revoked_at IS NULL;

CREATE TABLE environments (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    kind text NOT NULL CHECK (kind IN ('local', 'sandbox', 'staging')),
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, kind)
);
CREATE UNIQUE INDEX environments_one_default_idx ON environments (project_id) WHERE is_default;

ALTER TABLE findings
    ADD COLUMN cause text NOT NULL DEFAULT '',
    ADD COLUMN remediation text NOT NULL DEFAULT '';

COMMIT;
