# Project state

Updated: 2026-07-23

## Current milestone

The approved real-product plan in `docs/MVP_BUILD_PLAN.md` has completed its durable-data, real Stripe adapter, first durable-worker vertical, authentication/tenancy, persisted product-resource, resumable event-streaming, live product-route, and premium-marketing-motion slices. ParityLab is now a production-shaped, tenant-aware local MVP with a credential-gated real Stripe Sandbox path. It is not yet a deployed production service.

The next priority is the remaining real Stripe scenario executors, followed by hosted deployment and a real Stripe Sandbox acceptance run.

## What is actually implemented

### Backend and data truth

- Go 1.26 API with PostgreSQL 18 as the durable runtime adapter and a memory adapter for focused tests/demos.
- Checksum-validated, advisory-lock-protected automatic migrations through `000007_webhook_correlation`.
- Atomic run, event, report, idempotency, outbox, connection, webhook-deduplication, assertion, and reference-merchant-effect storage.
- Restart-safe run reads, idempotent request replay, webhook duplicate detection, and changed-body conflict rejection.
- Official Stripe Go SDK `v86.1.1` adapter. Only `sk_test_` and `rk_test_` secrets are accepted; live-shaped keys and live-mode webhook events are rejected.
- Server-side Stripe account validation, sanitized connection responses, AES-256-GCM encrypted connection secrets, and a real PaymentIntent-backed run endpoint.
- Stable Stripe idempotency/correlation parameters, integer minor-unit validation, persisted `pi_` evidence, and reproducible reports.
- Separate production worker command with PostgreSQL `FOR UPDATE SKIP LOCKED` claims, topic allowlists, leases, heartbeat, expiry recovery, retry backoff, terminal failure, and graceful shutdown.
- Versioned HMAC-signed reference-merchant contract and controlled healthy/duplicate/reorder/timeout/tamper relay behavior.
- Durable exactly-once merchant effects and a worker-written verification assertion in the persisted report.
- Webhook ingress atomically persists/deduplicates and enqueues a sanitized `stripe.webhook.received` projection. The durable worker consumes it, requires an exact PaymentIntent plus ParityLab-correlation match, derives the tenant only from the matched run, and atomically records terminal processing state, one API-visible run event, and one status-neutral report assertion.
- Webhook processing is restart-safe and idempotent: exact replay creates no second job/evidence; unsupported types become durable `ignored`, unmatched or missing-correlation events become durable `unmatched`, malformed internal jobs fail terminally, and transient storage failures retain bounded retry behavior. Raw signed bodies are neither persisted nor logged.
- Tenant/public-safe SSE reads persisted events in bounded PostgreSQL batches, uses stable sequence IDs, strictly validates `Last-Event-ID`, replays only missed evidence, emits retry/heartbeat/completion frames, and remains open for later webhook/worker evidence. Sliding per-stream write deadlines preserve the API server's ordinary slow-client timeout.

### Authentication, tenancy, and protected resources

- Real registration, login, session restoration, and logout. Registration atomically creates the encrypted user identity, organization, owner membership, project, 30-day retention setting, three default environments, opaque session, and audit entry.
- Passwords are hashed with Argon2id. Normalized emails are AES-256-GCM encrypted at rest and located through a keyed blind index; plaintext email and password are not stored.
- Session cookies are opaque, `HttpOnly`, `SameSite=Lax`, 24-hour cookies. Only SHA-256 token hashes are persisted; logout revokes the server-side session, and expired/revoked sessions return 401.
- Cookies are `Secure` by default. An explicit loopback-only insecure-cookie opt-in exists for local HTTP development and is rejected for non-loopback origins.
- Credentialed CORS is restricted to the configured web origin. Cookie-authenticated mutations enforce same-origin CSRF checks. Login uses generic invalid-credential responses, a dummy Argon2 verification for unknown accounts, bounded per-client/account throttling, and `Retry-After` on 429.
- Runs, events, reports, idempotency, Stripe connections, PaymentIntent execution, settings, environments, findings, notifications, and mutation audit records are scoped to the authenticated project. Cross-tenant identifiers return 404 and cannot be used to access another project’s Stripe connection or evidence.
- Project name/retention, selected environment, finding resolution/reopen, notification read/read-all, and sanitized connection lists are durable protected APIs. Tenant run creation also persists its finding and completion notification transactionally.
- Public seeded/demo records remain a separate `project_id IS NULL` boundary. They do not grant access to or blend with authenticated tenant records.

### Product and presentation

- Fifteen Next.js routes: marketing, demo, login, onboarding, dashboard, scenarios, runs/detail, findings, reports/detail, connections, environments, notifications, and settings.
- Existing API-backed scenarios, tenant-scoped runs, events, reports, findings, notifications, settings, environments, overview readiness, sanitized connection lists/validation, and PaymentIntent launch.
- Real registration/login/logout and protected-route session restoration use only the API-owned `HttpOnly` cookie; the web app does not store bearer/session tokens.
- Project settings, environment selection, finding resolution/reopen, and notification read/read-all controls persist through protected APIs and refresh shared shell state. Fixtures still use `Seeded preview` or `Simulated data` and are never labeled live.
- Functional command palette, navigation, account/notification controls, filters, exports, printing, copy actions, onboarding, connection checks, and mobile `More`/account menus.
- A 310svh pinned forensic marketing narrative driven by scroll progress, stateful event braid, chapter activation, premium hover/entry transitions, mobile linearization, and reduced-motion fallback.
- The live run detail bootstraps from the JSON ledger and then uses a credentialed native `EventSource`. Newly appended durable evidence is sequence-deduplicated, ordered, animated into the ledger, and accompanied by honest connecting/live/reconnecting status.
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
- `PARITYLAB_CONFIRM_FRESH=1 tests/scripts/auth-resource-restart.sh` — PASS on its isolated PostgreSQL/API/strict-Stripe-mock stack. It proves production-default and loopback cookie policy, registration sanitization, generic unknown/known login failures, throttling, protected Stripe execution, durable settings/environment/finding/notification mutations, cross-tenant 404s, CSRF rejection, expiration, API-restart persistence, logout revocation, and absence of password/Stripe/session secrets from API logs.
- Authenticated UI lint, unit tests, production build, E2E TypeScript, and static contract checks pass.
- Authenticated browser acceptance against the fresh PostgreSQL/API/strict-Stripe-mock stack — Chromium 17/17 passed in 11.6 seconds and WebKit 17/17 passed in 27.5 seconds across `auth-product.spec.ts`, `auth-security.spec.ts`, and `state-boundaries.spec.ts`. The dedicated browser stack and volumes were removed afterward.
- `pnpm audit --prod` — no known vulnerabilities after upgrading Next.js to 16.2.11 and overriding the inherited Sharp/libvips chain to 0.35.3 in response to the final audit.
- `govulncheck ./...` — no reachable Go vulnerabilities after upgrading `golang.org/x/text` to 0.39.0 (and its compatible `x/sync` dependency) to repair the scanner's reachable PostgreSQL parsing trace.
- `PARITYLAB_CONFIRM_FRESH=1 tests/scripts/webhook-consumer-restart.sh` — exit 0 with `webhook consumer restart contract passed for run_000004`; proved API-restart ingress durability, tenant-safe object/correlation matching, one visible event/assertion, worker-restart replay, changed-body conflict, terminal ignored/unmatched/poison outcomes, raw-body absence, secret-log absence, and scoped cleanup.
- `PARITYLAB_CONFIRM_FRESH=1 tests/scripts/sse-restart-reconnect.sh` — exit 0 with `SSE restart and reconnect contract passed for run_000004 through sequence 10`; proved exact replay 1–9, heartbeat/retry/completion framing, API restart, zero replay after reconnect at 9, live webhook append 10 on the same stream, tenant isolation, strict cursor errors, no duplicate sequences, and secret/raw-body absence.
- Production-built Chromium persisted-state suite — 8/8 passed, including the run detail appending a mocked durable event exactly once while retaining sequence order.

See `docs/VERIFICATION.md` and `docs/WORKSTREAMS/*.md` for the command ledger and lane-level evidence.

## Honest limitations / next gates

- No real Stripe account was contacted in this run because the user has not supplied local Sandbox credentials. The official SDK path is proven with a strict local Stripe server, not falsely reported as a live Stripe run.
- Only the PaymentIntent duplicate-delivery path has the complete real-adapter + durable-worker + merchant-assertion vertical. Remaining refund, subscription/Test Clock, reorder, timeout, and tamper scenario executors are not fully connected to real Stripe objects.
- Authentication currently supports one owner organization/project created at registration. Invitations, multi-project switching, password reset/email verification, MFA/passkeys, session-management UI, and automated encryption-key rotation are not implemented.
- The in-process login limiter is bounded and tested but is not yet a shared distributed limiter for a multi-instance deployment.
- WebKit authenticated acceptance is locally green. The broader full-route WebKit matrix, k6/load, hosted backup/recovery, penetration testing, and two clean hosted CI runs remain outstanding.
- No hosted web/API/worker/database deployment or public HTTPS Stripe webhook endpoint exists yet.

## Repository and mirror

- Active repository: `/Users/ayushkumarsingh/Documents/Codex/2026-07-18-here-is-the-stripe-internship-details/ParityLab`
- Desktop mirror target: `/Users/ayushkumarsingh/Desktop/PROJECTS/SideProjects/ParityLab`
- Latest committed green implementation checkpoint: `76d7da0` (`feat: correlate durable Stripe webhooks`).
- The Desktop mirror is refreshed from this checkpoint at handoff.
