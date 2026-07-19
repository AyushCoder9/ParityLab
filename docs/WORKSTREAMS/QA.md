# Verification workstream

Status: Phase 1, Phase 2 vertical slice, and Phase 4 product-shell acceptance green

## Scope

The verification lane owns executable evidence that ParityLab is durable, truthful, navigable, accessible, and safe. A polished seeded screen is not accepted as a live product unless its provenance is visible and its controls lead to implemented outcomes.

## Phase 1: durable data acceptance

`tests/scripts/persistence-restart.sh` creates only the dedicated Compose project `paritylab-persistence-contract`. It uses `infra/compose.test.yaml`, keeps PostgreSQL private to the test network, and exposes only the test API on port `18081`, then proves:

1. a fresh database reaches readiness after startup migrations;
2. a restricted test key validates against a strict Stripe mock and the API returns only sanitized connection metadata;
3. live-shaped keys and invalid currencies are rejected before a Stripe mutation;
4. a Stripe-shaped PaymentIntent request carries integer minor units, lowercase currency, stable `plcorr_<24hex>` metadata, and an idempotency key;
5. the resulting run, event evidence, assertion report, encrypted connection, idempotency record, and webhook receipt survive an API restart;
6. replay returns the same run with `Idempotent-Replayed: true`, while a new post-restart request can reuse the stored encrypted connection;
7. a Stripe-shaped sandbox webhook is accepted once and remains deduplicated after restart;
8. cleanup removes only the dedicated test project and its named test volumes.

The test intentionally requires `PARITYLAB_CONFIRM_FRESH=1`. It never operates on the default development Compose project or a host-wide Docker scope.

```bash
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/persistence-restart.sh
```

## Phase 4: live product acceptance

`tests/e2e/specs/mvp-product.spec.ts` is the browser-level product contract. It requires:

- real screens for login, onboarding, dashboard, scenarios, runs, findings, reports, connections, environments, notifications, and settings;
- route-based sidebar navigation instead of local placeholder toggles;
- routable seeded run/report details with visible seeded provenance;
- explicit `Engine unavailable — showing seeded preview` behavior when API requests fail;
- no fake `RUN_...` identifier after a failed API run creation;
- working notification, account, evidence, view-all, all-runs, and command-palette actions;
- no `Product demo` placeholder surface.

The accessibility matrix now covers every static product route and checks narrow-viewport overflow in addition to axe, keyboard, and reduced-motion gates.

`tests/e2e/specs/state-boundaries.spec.ts` distinguishes durable features from preview-only state:

- settings, notification resolution, finding resolution, and environment selection send no backend mutation and are visibly labeled browser/session-local;
- onboarding explains the encryption prerequisite and secure API handoff;
- a mocked connection flow proves the secret is posted once, cleared, absent from DOM/local storage, and replaced with a sanitized connection record;
- the frozen PaymentIntent route receives `connection_id`, integer `amount_minor`, and lowercase `currency`, then navigates to the returned persisted run;
- API-provided runs remain labeled live while seeded examples remain labeled seeded.

`tests/scripts/verify-openapi-contract.sh` prevents route/schema drift for the two Stripe endpoints, write-only secret input, sanitized connection response, minor-unit bounds, and currency format.

`tests/e2e/specs/stripe-vertical.spec.ts` is an opt-in, non-stubbed browser contract for the local production-shaped stack. With `PARITYLAB_STRIPE_VERTICAL_E2E=1`, it connects through the UI, creates the fixed 4200-USD PaymentIntent run through the live API and strict Stripe mock, opens the returned run, then confirms its run and balanced report through the API. The persistence harness separately proves that the same API records survive restart in PostgreSQL.

## CI gates

- `build-test`: lint, typecheck, unit tests, race-enabled Go tests, and production builds.
- `browser`: Chromium and WebKit application acceptance with retained failure evidence.
- `contract`: public API and signed webhook contracts.
- `persistence`: fresh PostgreSQL plus API-restart durability contract.
- `compose`: dependency configuration and startup health.

## Integration coordination

- Engine contract: `DATABASE_URL` selects PostgreSQL; migrations run before the API becomes healthy; unset retains the deterministic memory adapter.
- UI contract: offline state is `Seeded preview`; fetch failure is explicitly labeled; seeded identifiers use `seed_run_...`; commands and shell controls navigate or open functional surfaces.
- Stripe contract: `POST /v1/connections/stripe/validate` and `POST /v1/stripe/payment-intents`; outbound mock evidence requires `paritylab_correlation_id=plcorr_<24hex>` and `paritylab_scenario_id=checkout-duplicate`.
- Root owns final integration, full-suite execution, and `docs/STATE.md`.

## Commands

```bash
pnpm --filter @paritylab/e2e typecheck
pnpm --filter @paritylab/e2e exec playwright test specs/mvp-product.spec.ts --project=chromium --workers=1
pnpm --filter @paritylab/e2e exec playwright test specs/state-boundaries.spec.ts --project=chromium --workers=1
PARITYLAB_STRIPE_VERTICAL_E2E=1 pnpm --filter @paritylab/e2e exec playwright test specs/stripe-vertical.spec.ts --project=chromium --workers=1
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/persistence-restart.sh
tests/scripts/verify-openapi-contract.sh
tests/scripts/verify-config.sh
sh -n tests/scripts/*.sh
```
