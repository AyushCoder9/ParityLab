# Handoff

## Resume without chat history

```bash
cd /Users/ayushkumarsingh/Documents/Codex/2026-07-18-here-is-the-stripe-internship-details/ParityLab
git status --short --branch
make verify
```

Read in this order:

1. `AGENTS.md`
2. `docs/PROJECT_BRIEF.md`
3. `docs/STATE.md`
4. `docs/MVP_BUILD_PLAN.md`
5. `docs/IMPLEMENTATION_PLAN.md`
6. `docs/PLAN.md`
7. `docs/WORKSTREAMS/ENGINE.md`, `UI.md`, and `QA.md`
8. `docs/VERIFICATION.md`

These files capture the user's full-product requirement, approved ordering, exact implementation claims, tests, remaining limitations, and next gates. Do not infer completion from route names, migrations, or screenshots; use `docs/STATE.md` as the claim boundary.

## Local runtime

Copy `.env.example` to an ignored `.env.local` and generate local secrets. Never paste credentials into chat or commit them.

```bash
# Dependencies (set PARITYLAB_POSTGRES_PORT if 5432 is occupied)
docker compose -f infra/compose.yaml up -d postgres

# API
DATABASE_URL='postgres://paritylab:<password>@127.0.0.1:<port>/paritylab?sslmode=disable' \
PARITYLAB_MIGRATIONS_DIR=db/migrations \
PARITYLAB_ENCRYPTION_KEY='<base64-encoded-32-byte-key>' \
go run ./services/api/cmd/paritylab

# Durable worker in a second terminal
DATABASE_URL='postgres://paritylab:<password>@127.0.0.1:<port>/paritylab?sslmode=disable' \
PARITYLAB_MIGRATIONS_DIR=db/migrations \
PARITYLAB_SIGNING_SECRET='<at-least-16-byte-secret>' \
go run ./services/api/cmd/worker

# Web in a third terminal
NEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:8080 pnpm dev
```

The seeded tour needs no external credentials. A real Stripe path additionally needs an `sk_test_`/restricted `rk_test_` key and `whsec_` webhook secret stored only in local environment/secret storage. Stripe CLI `1.43.8` is installed at `/opt/homebrew/bin/stripe`.

Authentication is API-owned and cookie-based. Production/default startup issues `Secure`, `HttpOnly`, `SameSite=Lax` cookies. For loopback HTTP only, set `PARITYLAB_INSECURE_COOKIES=true`; startup rejects that opt-in when `WEB_ORIGIN` is not loopback. The browser and API origins must match the configured `WEB_ORIGIN` contract.

## Commands and proof

```bash
make test
make build                 # builds web, API, and worker
make verify
go vet ./...

# Destructive only to its dedicated test Compose project/volumes
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/persistence-restart.sh

# Destructive only to its separate dedicated auth test project/volumes
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/auth-resource-restart.sh
```

Browser tests can use external servers:

```bash
PARITYLAB_E2E_EXTERNAL_SERVERS=1 \
PARITYLAB_WEB_URL=http://127.0.0.1:3000 \
PARITYLAB_API_URL=http://127.0.0.1:8080 \
pnpm --dir tests/e2e exec playwright test --project=chromium
```

The opt-in Stripe vertical E2E requires the isolated strict Stripe mock stack documented in `docs/WORKSTREAMS/QA.md`; set `PARITYLAB_STRIPE_VERTICAL_E2E=1` only against that mock, never against a live endpoint by accident. The auth restart harness plus Chromium 17/17 and WebKit 17/17 authenticated browser runs are the authoritative integrated results for the new security/resource slice.

## Next implementation order

1. Add a dedicated `stripe.webhook.received` consumer that correlates Stripe events to active tenant runs and records processing state.
2. Convert replay-only SSE to long-lived database event streaming with reconnect/`Last-Event-ID` behavior.
3. Complete real Stripe refund and subscription/Test Clock scenario executors plus worker restart tests; expose the already-tested reorder, timeout, and tamper relay modes through persisted scenario configuration.
4. Add the next identity/operations layer: invitations or project switching, password recovery/verification, shared distributed throttling, key rotation, metrics/tracing, and administrative dead-letter replay.
5. Deploy web/API/worker/PostgreSQL with HTTPS webhook ingress, secret management, telemetry, backups, rate limits, and incident drills.
6. Run the final real Stripe Sandbox acceptance flow after the user places test credentials in ignored local files.

## Workspace truth

The active Git repository is the Documents/Codex path above. The Desktop path is a handoff mirror refreshed from green implementation checkpoint `68ca779`. If the two differ, prefer the Git repository whose `docs/STATE.md` names the newest green implementation commit.
