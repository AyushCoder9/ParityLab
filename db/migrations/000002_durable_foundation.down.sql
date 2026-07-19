BEGIN;

ALTER TABLE outbox
    DROP COLUMN last_error_code,
    DROP COLUMN locked_by,
    DROP COLUMN locked_at,
    DROP COLUMN available_at;

DROP TABLE audit_records;
DROP TABLE notifications;
DROP TABLE findings;
DROP TABLE assertions;
DROP TABLE job_leases;
DROP TABLE scenario_configurations;
DROP TABLE verification_targets;
DROP TABLE stripe_connections;
DROP TABLE memberships;
DROP TABLE users;
DROP TABLE reports;

ALTER TABLE run_events
    DROP COLUMN snapshot,
    DROP COLUMN is_duplicate,
    DROP COLUMN detail,
    DROP COLUMN title;

ALTER TABLE runs
    DROP COLUMN snapshot,
    DROP COLUMN recovered,
    DROP COLUMN finding_count,
    DROP COLUMN event_count,
    DROP COLUMN duration_ms,
    DROP COLUMN scenario_name;

DROP SEQUENCE run_number_seq;

COMMIT;
