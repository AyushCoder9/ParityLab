BEGIN;

ALTER TABLE runs
    ADD COLUMN stripe_correlation_id text;

CREATE INDEX runs_stripe_object_correlation_idx
    ON runs (stripe_object_id, stripe_correlation_id)
    WHERE stripe_object_id IS NOT NULL AND stripe_object_id <> '';

ALTER TABLE webhook_events
    ADD COLUMN stripe_created_at timestamptz,
    ADD COLUMN stripe_object_id text,
    ADD COLUMN object_status text,
    ADD COLUMN paritylab_correlation_id text,
    ADD COLUMN processing_status text NOT NULL DEFAULT 'pending'
        CHECK (processing_status IN ('pending', 'matched', 'unmatched', 'ignored')),
    ADD COLUMN correlated_run_id text REFERENCES runs(id) ON DELETE SET NULL,
    ADD COLUMN correlated_project_id uuid REFERENCES projects(id) ON DELETE SET NULL;

CREATE INDEX webhook_events_processing_idx
    ON webhook_events (processing_status, received_at)
    WHERE processing_status = 'pending';

CREATE TABLE stripe_webhook_evidence (
    stripe_event_id text PRIMARY KEY REFERENCES webhook_events(stripe_event_id) ON DELETE CASCADE,
    run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    project_id uuid REFERENCES projects(id) ON DELETE SET NULL,
    event_type text NOT NULL,
    stripe_object_id text NOT NULL,
    object_status text NOT NULL DEFAULT '',
    paritylab_correlation_id text NOT NULL DEFAULT '',
    run_event_id text NOT NULL UNIQUE REFERENCES run_events(id) ON DELETE CASCADE,
    recorded_at timestamptz NOT NULL DEFAULT now()
);

COMMIT;
