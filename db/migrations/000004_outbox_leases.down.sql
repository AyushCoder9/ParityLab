BEGIN;

DROP INDEX outbox_claim_idx;
ALTER TABLE outbox DROP COLUMN failed_at;

COMMIT;
