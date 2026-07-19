# Project state

Updated: 2026-07-19

## Current milestone

The approved real-product plan in `docs/MVP_BUILD_PLAN.md` has completed its durable-data, real Stripe adapter, first durable-worker vertical, live product-route, and premium-marketing-motion slices. ParityLab is now a production-shaped local MVP with a credential-gated real Stripe Sandbox path. It is not yet a deployed multi-tenant production service.

The next priority is authentication/organization ownership and persisted mutation APIs, followed by real Stripe webhook-to-run correlation, the remaining scenario executors, hosted deployment, and a real Stripe Sandbox acceptance run.

## What is actually implemented

### Backend and data truth

- Go 1.26 API with PostgreSQL 18 as the durable runtime adapter and a memory adapter for focused tests/demos.
- Checksum-validated, advisory-lock-protected automatic migrations through `000005_reference_merchant`.
- Atomic run, event, report, idempotency, outbox, connection, webhook-deduplication, assertion, and reference-merchant-effect storage.
- Restart-safe run reads, idempotent request replay, webhook duplicate detection, and changed-body conflict rejection.
- Official Stripe Go SDK `v86.1.1` adapter. Only `sk_test_` and `rk_test_` secrets are accepted; live-shaped keys and live-mode webhook events are rejected.
- Server-side Stripe account validation, sanitized connection responses, AES-256-GCM encrypted connection secrets, and a real PaymentIntent-backed run endpoint.
- Stable Stripe idempotency/correlation parameters, integer minor-unit validation, persisted `pi_` evidence, and reproducible reports.
- Separate production worker command with PostgreSQL `FOR UPDATE SKIP LOCKED` claims, topic allowlists, leases, heartbeat, expiry recovery, retry backoff, terminal failure, and graceful shutdown.
- Versioned HMAC-signed reference-merchant contract and controlled healthy/duplicate/reorder/timeout/tamper relay behavior.
- Durable exactly-once merchant effects and a worker-written verification assertion in the persisted report.
- Webhook ingress atomically persists/deduplicates and enqueues `stripe.webhook.received`; the current verification worker deliberately leaves that topic pending for a future correlated webhook consumer.

### Product and presentation

- Fifteen Next.js routes: marketing, demo, login, onboarding, dashboard, scenarios, runs/detail, findings, reports/detail, connections, environments, notifications, and settings.
- Existing API-backed scenarios, runs, events, reports, report-derived findings, overview readiness, connection validation, and PaymentIntent launch.
- Unsupported mutations are explicitly labeled browser/session-local; fixtures use `Seeded preview` or `Simulated data` and are never labeled live.
- Functional command palette, navigation, account/notification controls, filters, exports, printing, copy actions, onboarding, connection checks, and mobile `More`/account menus.
- A 310svh pinned forensic marketing narrative driven by scroll progress, stateful event braid, chapter activation, premium hover/entry transitions, mobile linearization, and reduced-motion fallback.
- Responsive Pixel 7 product layout, keyboard navigation, axe serious/critical checks, and visible provenance at all supported viewport sizes.

## Exact green evidence from this worktree

- `make verify && go vet ./... && git diff --check` — exit 0; includes frontend lint/typecheck/unit tests, all Go tests, 15-route production build, and both API/worker builds.
- `go test -race ./...` — all API, repository, Stripe adapter, verification, worker, and contract packages passed under the race detector.
- `PARITYLAB_CONFIRM_FRESH=1 tests/scripts/persistence-restart.sh` — exit 0 on a fresh isolated PostgreSQL + API + strict Stripe mock stack; passed for `run_000005` and cleaned its scoped containers/volumes.
- Chromium full suite against the integrated stack — 50/50 passed before the opt-in test was added.
- Mobile Chromium final suite — 50 passed, one intentional opt-in skip; targeted mobile portability 7/7 passed.
- Opt-in browser -> live API -> strict Stripe mock -> PostgreSQL ledger/report vertical — 1/1 passed.
- Strict Stripe mock and service tests cover authorization, live-key rejection, minor units, currency, stable correlation metadata, sanitized failures, and replay without a second Stripe call.
- Manual worker proof: `run_000034` and post-worker-restart `run_000035` each persisted `assert_reference_merchant_exactly_once`; two durable merchant effects remained after restart, and unhandled outbox topics stayed pending.
- `tests/scripts/verify-config.sh`, OpenAPI drift validation, shell syntax validation, and `git diff --check` pass.
- Desktop marketing and Pixel 7 dashboard screenshots were inspected; the prior hero whitespace issue is gone and the mobile product navigation is complete.

See `docs/VERIFICATION.md` and `docs/WORKSTREAMS/*.md` for the command ledger and lane-level evidence.

## Honest limitations / next gates

- No real Stripe account was contacted in this run because the user has not supplied local Sandbox credentials. The official SDK path is proven with a strict local Stripe server, not falsely reported as a live Stripe run.
- Authentication, sessions, membership enforcement, and tenant authorization are not implemented. The current fixed local workspace/project is for local MVP verification only.
- Findings, notifications, settings, and environment changes remain explicitly session/browser-local until their protected mutation APIs exist.
- `stripe.webhook.received` is durably queued but not yet correlated to a run or consumed by a dedicated webhook-domain worker.
- Only the PaymentIntent duplicate-delivery path has the complete real-adapter + durable-worker + merchant-assertion vertical. Remaining refund, subscription/Test Clock, reorder, timeout, and tamper scenario executors are not fully connected to real Stripe objects.
- The SSE endpoint replays persisted events and completion; it is not yet a database-backed long-lived append subscription with `Last-Event-ID` recovery.
- WebKit is configured in CI but was not installed/run locally. k6/load, hosted backup/recovery, penetration, and two clean hosted CI runs remain outstanding.
- No hosted web/API/worker/database deployment or public HTTPS Stripe webhook endpoint exists yet.

## Repository and mirror

- Active repository: `/Users/ayushkumarsingh/Documents/Codex/2026-07-18-here-is-the-stripe-internship-details/ParityLab`
- Desktop mirror target: `/Users/ayushkumarsingh/Desktop/PROJECTS/SideProjects/ParityLab`
- Latest committed plan checkpoint before this slice: `5288813` (`docs: confirm real-product MVP build plan`).
- The integrated implementation commit and mirror refresh are the root agent's next handoff actions.
