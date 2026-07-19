BEGIN;

CREATE TABLE reference_merchant_effects (
    effect_id text PRIMARY KEY,
    last_sequence integer NOT NULL CHECK (last_sequence > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMIT;
