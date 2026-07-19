# UI workstream

Status: Phase 4 product route slice implemented; backend expansion remains active

## Delivered

- Replaced the dashboard's local placeholder-state navigation with a shared Next.js App Router shell and real links.
- Added complete routed screens for `/login`, `/onboarding`, `/dashboard`, `/scenarios`, `/runs`, `/runs/[id]`, `/findings`, `/reports`, `/reports/[id]`, `/connections`, `/environments`, `/notifications`, and `/settings`.
- Added live API readers for engine health, overview, scenarios, run list/detail/events, and reports. The dashboard and run ledger use API data when available.
- Wired the live engine scenario catalog to `POST /v1/runs`; successful creation navigates directly to persisted run evidence and failed creation never invents an ID.
- Added secure Stripe Sandbox validation through `POST /v1/connections/stripe/validate`. The secret is never stored/logged by UI code, is rejected locally when live-mode shaped, and is cleared immediately after each attempt; only the sanitized connection record is rendered.
- Wired validated connections to the real PaymentIntent vertical slice at `POST /v1/stripe/payment-intents` using integer minor units, ISO currency, and an idempotency key, then routes to the returned persisted run/report evidence.
- Reports now mix current API runs with clearly labeled seeded examples; findings are derived from available run-report APIs and session-only resolution is labeled explicitly.
- Kept seeded records available as an explicitly labeled preview. API failure renders `Engine unavailable — showing seeded preview`; fixture rows say `Seeded`, and live rows say `Live`.
- Removed the demo's misleading `RUN_01J8Z4` failure fallback. Failed API creation now displays `SEED_PREVIEW` under the existing `Simulated data` label.
- Wired visible product controls: navigation and active route state, notification destination and read state, account menu, command-palette filtering/navigation and keyboard shortcut, overview drill-downs, scenario search/category filter, related-run filters, run search/status filters, run-ID copy, JSON evidence/report exports, print reports, finding triage and rerun, engine connection check, environment selection, onboarding template copy, and browser-preview settings save.
- Added useful route, skeleton, explicit failure, filter-empty, and connected/seeded states without presenting fixtures as live Stripe evidence.
- Reworked the Pixel 7 application shell: four primary route links remain directly visible, a fifth `More` control opens every remaining product destination plus account settings, and a dedicated mobile account menu remains keyboard/screen-reader accessible.
- Kept provenance visible on compact demo layouts by replacing the generic mobile `Sandbox` badge with the explicit `Simulated data` label.
- Preserved the Optical Ledger visual system, keyboard focus, reduced-motion behavior, print treatment, and responsive bottom navigation.
- Began the Phase 5 marketing pass with a 310svh pinned forensic narrative, scroll-progress-driven braid, four evidence chapters, state transitions from healthy to fault to verified, section-entry emphasis, stronger hover response, and light product route motion. Motion is transform/filter based, avoids layout-property animation, collapses to a linear mobile story, and resolves immediately under reduced motion.

## Verification

- `pnpm --filter @paritylab/web lint` — pass, strict TypeScript/no emit.
- `pnpm --filter @paritylab/web test` — pass, 3/3 deterministic simulation tests.
- `pnpm --filter @paritylab/web build` — pass on Next.js 16.2.10; 15 routes generated, with seeded report/run detail routes served dynamically.
- `pnpm --filter @paritylab/e2e typecheck` — pass.
- Production route smoke check — HTTP 200 for all 13 product/access routes and seeded run/report detail routes.
- `mvp-product.spec.ts` Chromium acceptance: all 18 checks passed across the run history; the complete cold-development run had one-time dynamic-route compilation timeouts, and the affected drill-down test passed after compilation. Isolated production checks for seeded drill-down and outage behavior passed 2/2. This is a dev-server cold-compile timing artifact, not an assertion defect.
- Production Chromium acceptance after the second slice: `mvp-product.spec.ts` plus `marketing.spec.ts` passed 21/21.
- Accessibility pass: 15/16 initially passed; the only failure was insufficient contrast on pending onboarding steps. The color token was corrected and the targeted onboarding axe rerun passed 1/1. The complete suite already confirmed reduced-motion behavior and zero narrow-viewport overflow across all product routes.
- Mobile Chromium route/action rerun after the first responsive audit: 18/21 passed; the genuine UI failure was only three visible product links and is corrected to four plus the functional More menu. The other two failures required the stale local API process to report engine online/create a run and are environment-dependent, not replaced with misleading UI state.

## Honest limitations

- Authentication, organization persistence, connection mutation, findings mutation, notification persistence, and settings persistence APIs do not exist yet. Those screens visibly identify local/seeded state; settings use browser local storage and triage/notification actions are session-local.
- Real Stripe Sandbox success remains credential-gated. The secure connection and PaymentIntent UI contracts are implemented, but a successful end-to-end browser run requires `PARITYLAB_ENCRYPTION_KEY` plus an `rk_test_...`/`sk_test_...` credential supplied by the user.
- The API currently exposes overview/runs/evidence data but not every Phase 4 resource. Seeded records stay visible alongside live run records and are labeled per row.
- SSE-driven live run insertion and durable mutation toasts remain. Auth, workspace, environment, finding-resolution, notification, and settings mutations still need backend contracts.

## Integration notes

- Set `NEXT_PUBLIC_PARITYLAB_API_URL` before `next build`; browser API calls use that injected origin. Default remains `http://127.0.0.1:8080`.
- Seeded identifiers use the `seed_run_` prefix. Do not return that prefix from live endpoints.
- The shared product shell lives in `apps/web/src/components/app-shell.tsx`; routed product views live in `apps/web/src/components/product-pages.tsx`; API/fixture boundaries live in `apps/web/src/lib/api.ts` and `apps/web/src/lib/product-data.ts`.
