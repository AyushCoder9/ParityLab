BEGIN;

DROP TABLE environments;
DROP TABLE sessions;

ALTER TABLE findings
    DROP COLUMN remediation,
    DROP COLUMN cause;

ALTER TABLE projects
    DROP COLUMN updated_at,
    DROP COLUMN retention_days;

ALTER TABLE users
    DROP COLUMN updated_at,
    DROP COLUMN password_hash,
    DROP COLUMN email_ciphertext;

COMMIT;
