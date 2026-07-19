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
- WebKit and k6 were not run locally.
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
- `specs/state-boundaries.spec.ts --project=chromium --workers=1` — 7/7 pass, covering browser/session-local non-mutations, sanitized Stripe connection handoff, frozen PaymentIntent request wiring, and live-versus-seeded labeling.
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
