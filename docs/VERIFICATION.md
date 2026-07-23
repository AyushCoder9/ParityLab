# Verification ledger

Date: 2026-07-18

## Environment

- macOS host, Node.js 26, pnpm 10.13.1.
- Go was not installed on the host; all Go checks used the official `golang:1.26-alpine` image.
- Web production server: `http://127.0.0.1:3100`.
- Isolated API container: `http://127.0.0.1:18080` because another user process already owned port 8080.

## Final commands

```bash
NEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:18080 pnpm build
pnpm lint
pnpm typecheck
pnpm test
pnpm audit --prod
docker compose -f infra/compose.yaml config -q
bash tests/scripts/verify-config.sh
```

Go checks were executed through the pinned container with the repository mounted at `/src`:

```bash
go test ./...
go vet ./...
go build ./...
```

Integrated browser gate:

```bash
PARITYLAB_WEB_URL=http://127.0.0.1:3100 \
PARITYLAB_API_URL=http://127.0.0.1:18080 \
PARITYLAB_E2E_EXTERNAL_SERVERS=1 \
pnpm --filter @paritylab/e2e exec playwright test \
  --project=chromium --project=mobile-chrome
```

Live webhook contract:

```bash
PARITYLAB_CONTRACT_API_URL=http://host.docker.internal:18080 \
go test -v ./tests/contracts
```

## Repair loop

1. Initial UI build and deterministic tests passed.
2. First E2E run passed 21/28 and found invalid ARIA labels, incomplete table roles, an unlabeled SVG, and desktop-only mobile assertions.
3. Product semantics and viewport-aware assertions were corrected; the next run passed 27/28.
4. The compact mobile brand semantic was repaired; the next run passed 28/28.
5. Dependency audit found high-severity advisories in the original Next.js and Playwright pins.
6. Next.js, React, Playwright, Axe, and PostCSS were upgraded to patched current releases; audit became clean and 28/28 E2E remained green.
7. The UI was connected to the Go engine through the shared contract package; integrated assertions prove a real API-created run ID and engine-online state.
8. Two consecutive final integrated browser runs passed 28/28 on the exact connected product tree.

## Deliberately not claimed

- No real Stripe key was available, so Stripe Sandbox network calls were not executed.
- At this 2026-07-18 checkpoint, WebKit and k6 were not run locally. The later 2026-07-23 auth slice below includes a successful focused WebKit run; k6 remains outstanding.
- The complete Compose dependency stack was not started against this shared Docker host.

## Real-product expansion ledger

Date: 2026-07-19

The confirmed MVP plan adds two new non-negotiable verification surfaces:

```bash
pnpm --filter @paritylab/e2e typecheck
docker compose -f infra/compose.test.yaml config --quiet
tests/scripts/verify-config.sh
sh -n tests/scripts/*.sh
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/persistence-restart.sh
pnpm --filter @paritylab/e2e exec playwright test \
  specs/mvp-product.spec.ts --project=chromium --workers=1
```

Local results after the engine and UI integration slices landed:

- `pnpm --filter @paritylab/e2e typecheck` — pass.
- `docker compose -f infra/compose.test.yaml config --quiet` — pass.
- `tests/scripts/verify-config.sh` — pass; default and isolated test Compose models render.
- `sh -n tests/scripts/*.sh` — pass.
- `git diff --check` — pass.
- `specs/mvp-product.spec.ts --project=chromium --workers=1` against web `localhost:3200` and PostgreSQL-backed API `127.0.0.1:18080` — 18/18 pass.
- `specs/api.spec.ts --project=chromium --workers=1` against the same API — 3/3 pass, including the new run-ledger list contract.
- At the 2026-07-19 checkpoint, `specs/state-boundaries.spec.ts --project=chromium --workers=1` passed 7/7 for the then-browser-local mutation boundary, sanitized Stripe connection handoff, frozen PaymentIntent request wiring, and live-versus-seeded labeling. The 2026-07-23 auth/resource slice supersedes that browser-local boundary with protected durable APIs.
- `tests/scripts/verify-openapi-contract.sh` — pass for both Stripe endpoints, write-only secret input, sanitized output, integer minor units, and lowercase currency.
- `PARITYLAB_CONFIRM_FRESH=1 tests/scripts/persistence-restart.sh` — pass; printed `persistence restart contract passed for run_000005`, exit 0.
- Targeted mobile portability rerun against web `127.0.0.1:3200`, PostgreSQL-backed API `127.0.0.1:18082`, and Stripe mock `127.0.0.1:12112` — 7/7 pass.
- Full `mobile-chrome` suite against that stack — 50 passed, 1 intentionally skipped credential/stack-gated vertical test.
- `PARITYLAB_STRIPE_VERTICAL_E2E=1 specs/stripe-vertical.spec.ts --project=chromium --workers=1` against that stack — 1/1 pass. The browser validated a sanitized mock Sandbox connection, created the fixed 4200-USD Stripe-mock run through the live API, opened its evidence route, and confirmed its persisted run/report API contract.
- Final E2E TypeScript typecheck, configuration/OpenAPI validation, and owned-file `git diff --check` — pass.

Harness repair history before the final pass:

1. PostgreSQL was removed from host-port exposure after a collision with an unrelated service.
2. API readiness was corrected from an accidental `HEAD` probe to the real `GET /healthz` contract.
3. Failure paths now print scoped Compose logs before cleanup.
4. The strict Stripe mock exposed the correlation metadata decision; the frozen contract is `paritylab_correlation_id=plcorr_<24hex>` plus `paritylab_scenario_id=checkout-duplicate`, which remains stable across concurrent/replayed allocation.
5. Concurrent invocations against the fixed Compose project were stopped; the root agent owned the final single authoritative run.

The persistence test is isolated from development data. Its destructive operations are limited to the explicit Compose project `paritylab-persistence-contract` and its dedicated volumes, and require `PARITYLAB_CONFIRM_FRESH=1`.

## Authentication, tenancy, and protected resources

Date: 2026-07-23

- `PARITYLAB_CONFIRM_FRESH=1 tests/scripts/auth-resource-restart.sh` — exit 0 with `auth security and restart contract passed`; the dedicated containers, network, and volumes were absent after cleanup.
- `auth-product.spec.ts`, `auth-security.spec.ts`, and `state-boundaries.spec.ts` against the fresh PostgreSQL/API/strict-Stripe-mock stack — Chromium 17/17 passed in 11.6 seconds.
- The same three suites using Playwright WebKit 26.5 build 2311 — WebKit 17/17 passed in 27.5 seconds. The pinned WebKit runtime was installed after an initial launch-only missing-runtime result; no application assertion failed in that initial attempt.

The auth restart run used only the dedicated Compose project `paritylab-auth-security-contract`, a fresh PostgreSQL database, the rebuilt API, and the strict local Stripe server. Its detailed coverage was:

- production/default `Secure`, `HttpOnly`, `SameSite=Lax`, 24-hour session-cookie attributes and the explicit loopback HTTP cookie mode;
- sanitized account registration, owner organization/project/default-environment creation, and no password/hash in the response;
- anonymous denial before protected Stripe validation/execution;
- identical public error content for known/unknown invalid login, bounded burst throttling, 429 `Retry-After`, and dummy-hash behavior through focused auth tests;
- durable project name/retention, selected environment, finding resolution, notification read state, sanitized Stripe connection, and tenant PaymentIntent execution;
- cross-tenant 404 for another project’s connection, environment, finding, and notification;
- 403 for a foreign-Origin cookie mutation;
- 401 for an expired session, persistence of tenant state across API restart, and 401 for the explicitly revoked logout cookie;
- API-log absence of the password sentinel, Stripe test secret, and both opaque session tokens.

Supporting gates for the slice:

```bash
pnpm --filter @paritylab/web lint
pnpm --filter @paritylab/web test
NEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:8080 pnpm --filter @paritylab/web build
pnpm --filter @paritylab/e2e typecheck
tests/scripts/verify-config.sh
tests/scripts/verify-openapi-contract.sh
sh -n tests/scripts/*.sh
git diff --check
```

The final production audit initially identified newly published advisories affecting Next.js 16.2.10 and Next's inherited Sharp 0.34.5/libvips path. Next.js was upgraded to 16.2.11 and the workspace now overrides Sharp to 0.35.3; `pnpm audit --prod` then returned `No known vulnerabilities found`. `govulncheck ./...` also found a reachable `golang.org/x/text` 0.29.0 issue through PostgreSQL connection parsing; upgrading `x/text` to 0.39.0 and the compatible `x/sync` dependency produced `No vulnerabilities found`. `make verify`, `go vet ./...`, `go test -race ./...`, configuration/OpenAPI checks, shell syntax, and diff checks all remained green after those dependency repairs.

The authenticated UI static gates, E2E TypeScript validation, isolated auth/restart contract, and integrated Chromium/WebKit browser contracts are green. No real Stripe account was contacted; the Stripe portion used the strict local mock. Remaining real scenario executors and hosted deployment are still unfinished.

## Durable webhook consumer

Date: 2026-07-23

```bash
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/webhook-consumer-restart.sh
```

The dedicated `paritylab-webhook-consumer-contract` stack passed with `webhook consumer restart contract passed for run_000004`. It proved signed-ingress durability across an API restart, exact tenant-safe PaymentIntent/correlation matching, one visible event/evidence/assertion effect, worker-restart and delivery-replay idempotency, changed-body conflict handling, terminal ignored and unmatched classifications, poison-job isolation, and absence of raw webhook bodies, fixture PII, and secrets from persisted projections and logs.

The PostgreSQL worker integration passed 2/2, including restart/replay behavior. Targeted worker and repository tests, `make verify`, `go vet ./...`, `go test -race ./...`, production dependency audits, `govulncheck`, configuration/OpenAPI validation, shell syntax, and `git diff --check` were all green after the consumer landed.

## Resumable long-lived SSE

Date: 2026-07-23

```bash
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/sse-restart-reconnect.sh
```

The dedicated `paritylab-sse-reconnect-contract` stack passed with `SSE restart and reconnect contract passed for run_000004 through sequence 10`. The first authenticated connection replayed exact stable IDs 1–9, emitted `retry: 2000`, one completion frame, valid JSON data, and a heartbeat without closing. After disconnect and API restart, `Last-Event-ID: 9` replayed zero events; the reconnected stream remained open and received the later webhook-correlated sequence 10. Combined IDs were exactly 1–10 with no duplicates.

The same harness proved anonymous and foreign-tenant 404s; strict 400 responses for malformed, duplicate, and ahead cursors; persisted JSON/API parity; secret/raw-body absence from SSE and all service logs; and fully scoped cleanup. This final evidence was collected only after replacing the API's incompatible absolute stream timeout with refreshed per-SSE write deadlines while retaining the ordinary global slow-client timeout.

The production-built Chromium persisted-state suite passed 8/8. Its run-detail contract rendered one JSON-bootstrap event plus one streamed event as two ordered ledger rows, with the appended sequence visible exactly once; all prior protected-state and Stripe-handoff tests remained green. Supporting engine, PostgreSQL, HTTP, and API-server tests passed normally and under the focused race detector; `go vet`, configuration/OpenAPI validation, shell syntax, and diff checks were green.
