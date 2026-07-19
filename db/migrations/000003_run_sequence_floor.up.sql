BEGIN;

SELECT setval(
    'run_number_seq',
    GREATEST(
        3,
        COALESCE((SELECT max(substring(id FROM 5)::bigint) FROM runs), 0)
    ),
    true
);

COMMIT;
