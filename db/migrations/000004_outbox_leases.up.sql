BEGIN;

ALTER TABLE outbox ADD COLUMN failed_at timestamptz;

CREATE INDEX outbox_claim_idx
    ON outbox (available_at, created_at)
    WHERE published_at IS NULL AND failed_at IS NULL;

COMMIT;
