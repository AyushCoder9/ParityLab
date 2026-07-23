# UI workstream

Status: authenticated product-resource slice implemented; Chromium and WebKit integrated acceptance green

## Delivered

- Replaced the dashboard's local placeholder-state navigation with a shared Next.js App Router shell and real links.
- Added complete routed screens for `/login`, `/onboarding`, `/dashboard`, `/scenarios`, `/runs`, `/runs/[id]`, `/findings`, `/reports`, `/reports/[id]`, `/connections`, `/environments`, `/notifications`, and `/settings`.
- Added live API readers for engine health, overview, scenarios, run list/detail/events, and reports. The dashboard and run ledger use API data when available.
- Wired the live engine scenario catalog to `POST /v1/runs`; successful creation navigates directly to persisted run evidence and failed creation never invents an ID.
- Added secure Stripe Sandbox validation through `POST /v1/connections/stripe/validate`. The secret is never stored/logged by UI code, is rejected locally when live-mode shaped, and is cleared immediately after each attempt; only the sanitized connection record is rendered.
- Wired validated connections to the real PaymentIntent vertical slice at `POST /v1/stripe/payment-intents` using integer minor units, ISO currency, and an idempotency key, then routes to the returned persisted run/report evidence.
- Reports now mix current API runs with clearly labeled seeded examples; authenticated findings are loaded from the protected project API and resolve/reopen durably.
- Kept seeded records available as an explicitly labeled preview. API failure renders `Engine unavailable — showing seeded preview`; fixture rows say `Seeded`, and live rows say `Live`.
- Removed the demo's misleading `RUN_01J8Z4` failure fallback. Failed API creation now displays `SEED_PREVIEW` under the existing `Simulated data` label.
- Wired visible product controls: navigation and active route state, durable notification read state, account/logout menu, command-palette filtering/navigation and keyboard shortcut, overview drill-downs, scenario search/category filter, related-run filters, run search/status filters, run-ID copy, JSON evidence/report exports, print reports, durable finding triage/rerun, engine connection check, durable environment selection, onboarding template copy, and persisted project settings.
- Added useful route, skeleton, explicit failure, filter-empty, and connected/seeded states without presenting fixtures as live Stripe evidence.
- Reworked the Pixel 7 application shell: four primary route links remain directly visible, a fifth `More` control opens every remaining product destination plus account settings, and a dedicated mobile account menu remains keyboard/screen-reader accessible.
- Kept provenance visible on compact demo layouts by replacing the generic mobile `Sandbox` badge with the explicit `Simulated data` label.
- Preserved the Optical Ledger visual system, keyboard focus, reduced-motion behavior, print treatment, and responsive bottom navigation.
- Began the Phase 5 marketing pass with a 310svh pinned forensic narrative, scroll-progress-driven braid, four evidence chapters, state transitions from healthy to fault to verified, section-entry emphasis, stronger hover response, and light product route motion. Motion is transform/filter based, avoids layout-property animation, collapses to a linear mobile story, and resolves immediately under reduced motion.
- Replaced local-preview access with real registration, login, logout, session restoration, and protected-route redirects. The API-owned `HttpOnly` cookie is never exposed to JavaScript or copied into browser storage, and a session-check outage renders an explicit blocked state instead of a local identity.
- Rendered the authenticated email, organization, role, project, selected environment, unread-notification count, and open-finding count from protected API responses in the product shell.
- Replaced browser-only settings, environment selection, finding resolution, and notification read state with protected API reads and mutations. Successful project updates refresh the shared session view, and resource changes refresh shell counters without a page reload.
- Connections now load the authenticated project’s persisted sanitized connection records before offering a new secret handoff. Stripe secrets remain memory-only in the form and are cleared after every attempt.
- Corrected onboarding copy to describe the persisted user/organization/project transaction and server-side encrypted connection setup; it no longer claims the workspace is browser-only.

## Verification

- `pnpm --filter @paritylab/web lint` — pass, strict TypeScript/no emit.
- `pnpm --filter @paritylab/web test` — pass, 3/3 deterministic simulation tests.
- `pnpm --filter @paritylab/web build` — pass on Next.js 16.2.11; 15 routes generated, with seeded report/run detail routes served dynamically.
- `pnpm --filter @paritylab/e2e typecheck` — pass.
- Production route smoke check — HTTP 200 for all 13 product/access routes and seeded run/report detail routes.
- `mvp-product.spec.ts` Chromium acceptance: all 18 checks passed across the run history; the complete cold-development run had one-time dynamic-route compilation timeouts, and the affected drill-down test passed after compilation. Isolated production checks for seeded drill-down and outage behavior passed 2/2. This is a dev-server cold-compile timing artifact, not an assertion defect.
- Production Chromium acceptance after the second slice: `mvp-product.spec.ts` plus `marketing.spec.ts` passed 21/21.
- Accessibility pass: 15/16 initially passed; the only failure was insufficient contrast on pending onboarding steps. The color token was corrected and the targeted onboarding axe rerun passed 1/1. The complete suite already confirmed reduced-motion behavior and zero narrow-viewport overflow across all product routes.
- Mobile Chromium route/action rerun after the first responsive audit: 18/21 passed; the genuine UI failure was only three visible product links and is corrected to four plus the functional More menu. The other two failures required the stale local API process to report engine online/create a run and are environment-dependent, not replaced with misleading UI state.
- Authenticated UI static verification: `pnpm --filter @paritylab/web lint` passed; `pnpm --filter @paritylab/web test` passed 3/3; `NEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:8080 pnpm --filter @paritylab/web build` passed with all 15 routes generated.
- Fresh-stack authenticated Playwright acceptance across `auth-product.spec.ts`, `auth-security.spec.ts`, and `state-boundaries.spec.ts`: Chromium 17/17 passed in 11.6 seconds; WebKit 17/17 passed in 27.5 seconds.
- The final production audit found newly published Next.js and Sharp/libvips advisories. Next.js was upgraded to 16.2.11 and Sharp was pinned to 0.35.3; `pnpm audit --prod` now reports no known vulnerabilities, and the full static/build verification remained green.

## Honest limitations

- Real Stripe Sandbox success remains credential-gated. The secure connection and PaymentIntent UI contracts are implemented, but a successful end-to-end browser run requires `PARITYLAB_ENCRYPTION_KEY` plus an `rk_test_...`/`sk_test_...` credential supplied by the user.
- The current authenticated UI covers one owner organization/project. Invitations, project switching, password recovery/verification, MFA/passkeys, and session inventory are not yet product screens.
- SSE-driven live run insertion and richer durable mutation feedback remain. The bundled reference merchant is local-only, and the public demo plus seeded run/report fallbacks remain explicitly labeled as simulations or seeded evidence.

## Integration notes

- Set `NEXT_PUBLIC_PARITYLAB_API_URL` before `next build`; browser API calls use that injected origin. Default remains `http://127.0.0.1:8080`.
- Set API `WEB_ORIGIN` to the exact browser origin. Credentialed requests use `credentials: "include"`; local plain-HTTP development additionally requires the API’s loopback-only `PARITYLAB_INSECURE_COOKIES=true` option.
- Seeded identifiers use the `seed_run_` prefix. Do not return that prefix from live endpoints.
- The shared product shell lives in `apps/web/src/components/app-shell.tsx`; routed product views live in `apps/web/src/components/product-pages.tsx`; API/fixture boundaries live in `apps/web/src/lib/api.ts` and `apps/web/src/lib/product-data.ts`.
- Session restoration/protected redirects live in `apps/web/src/components/auth-context.tsx`; registration and login live in `apps/web/src/components/access-pages.tsx`. Never add a local identity fallback or place the opaque cookie value in browser-accessible storage.
