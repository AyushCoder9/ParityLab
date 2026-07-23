BEGIN;

DROP TABLE stripe_webhook_evidence;

DROP INDEX webhook_events_processing_idx;

ALTER TABLE webhook_events
    DROP COLUMN correlated_project_id,
    DROP COLUMN correlated_run_id,
    DROP COLUMN processing_status,
    DROP COLUMN paritylab_correlation_id,
    DROP COLUMN object_status,
    DROP COLUMN stripe_object_id,
    DROP COLUMN stripe_created_at;

DROP INDEX runs_stripe_object_correlation_idx;

ALTER TABLE runs
    DROP COLUMN stripe_correlation_id;

COMMIT;
