# ParityLab real-product MVP build plan

Status: confirmed by the user on 2026-07-19; implementation active.

## Decision

ParityLab will become a real, sandbox-only Stripe reliability product. A user connects a Stripe Sandbox and a merchant verification target, runs a failure scenario against real Stripe test objects and real signed webhook deliveries, watches the run progress from persisted events, and receives an evidence report proving whether Stripe state and merchant state converged.

The build order is fixed by product risk:

1. Real backend, persistence, and Stripe Sandbox execution.
2. Complete product workflows and live UI data.
3. Award-level marketing motion and premium product polish.

Visual spectacle must never substitute for a working control, stored record, or truthful state.

## Current-state audit

| Area | Present now | Missing before a real MVP |
| --- | --- | --- |
| Go control plane | Health, scenario, run, event, report, SSE-shaped response, and signed webhook routes | Durable repositories, actual Stripe API calls, durable orchestration, auth/tenancy, target connection, retries, deployment |
| PostgreSQL | Initial schema and Compose service | Runtime connection, migrations on startup/deploy, repository implementation, transactions, outbox publisher, retention |
| Stripe | Signature verification and sandbox/live-mode rejection | Stripe SDK adapter, Sandbox connection validation, real PaymentIntent/Billing objects, webhook registration/listener, test clocks |
| Simulation | Real API run creation is attempted | Playback is driven by local TypeScript fixtures, not API events; API failure silently falls back to a fake run ID |
| Dashboard | Polished overview shell and engine health check | Overview values are seeded; most navigation opens a placeholder; several buttons only close UI or do nothing |
| Product screens | Home, demo, dashboard, 404 | Onboarding, connections, environments, scenarios, runs, run detail, findings, reports, notifications, settings |
| Authentication | None | Real session, protected app routes, organization/project ownership, audit trail |
| Deployment | Local scripts and infrastructure definitions | Hosted web/API/worker/database, HTTPS webhook URL, secrets management, migrations, health checks |
| Motion | Event-braid SVG motion, state cycling, basic hover/press states, smooth anchor scroll | Scroll choreography, pinned narrative, section transitions, kinetic typography, route/panel transitions, richer hover response |

The current product is therefore a tested deterministic prototype, not a fully wired production MVP.

## Product workflow

### First-run onboarding

1. Sign in and create a workspace/project.
2. Connect a Stripe Sandbox using a restricted sandbox key where possible.
3. Validate the key server-side and display the connected account/sandbox identity.
4. Configure the ParityLab webhook endpoint and verify a signed test event.
5. Choose the bundled reference merchant target or add an external verification target.
6. Run a connection check that proves Stripe, webhook ingress, target, worker, and database connectivity.

### Real verification run

1. User chooses a scenario and its parameters.
2. API creates an idempotent persisted run and queues a durable orchestration job.
3. Worker creates real Stripe Sandbox objects with correlation metadata.
4. Stripe emits real events to ParityLab's public/local webhook ingress.
5. The fault relay duplicates, delays, reorders, drops, or tampers with the delivery sent to the merchant target.
6. ParityLab collects target responses and queries current Stripe state.
7. Deterministic assertions compare Stripe, ingress, and merchant state.
8. Events, findings, assertion evidence, and an immutable report are persisted.
9. The browser receives progress through SSE and renders the same persisted evidence that APIs and reports expose.

### MVP scenarios

- Duplicate checkout submission: one PaymentIntent and one merchant order.
- Duplicate webhook delivery: two deliveries, exactly one business effect.
- Out-of-order webhook delivery: older event cannot regress current state.
- Endpoint timeout and retry: recovery without event loss or double processing.
- Tampered payload: invalid signature produces no merchant mutation.
- Partial refund convergence: integer minor-unit values agree across systems.
- Subscription renewal recovery: Stripe Test Clock advances a sandbox subscription and verifies entitlement convergence.

## Backend architecture

```text
Next.js web
    |
    v
Go API/control plane ---- PostgreSQL (source of truth)
    |                         | runs, events, findings
    |                         | connections, encrypted secrets
    |                         | idempotency, audit, outbox
    v                         v
Durable worker <-------- outbox/job claims
    |
    +---- Stripe Sandbox API
    +---- signed Stripe webhook ingress
    +---- controlled fault relay
    +---- merchant verification target
```

### Backend implementation slices

1. Add a `Repository` port and PostgreSQL adapter using `pgx`; use transactions for run state, event append, idempotency, webhook deduplication, and outbox insertion.
2. Expand migrations for users, memberships, Stripe connections, verification targets, scenario configurations, job leases, assertions, findings, reports, notifications, and audit records.
3. Encrypt connected secrets at rest with an application encryption key; never return or log their plaintext.
4. Add a Stripe adapter using the official Go SDK. Reject `sk_live_`, `rk_live_`, `pk_live_`, and `livemode: true` at every boundary.
5. Build a durable worker with lease/heartbeat/retry semantics. A process restart must resume or safely fail an incomplete run.
6. Make webhook ingestion fast and asynchronous: verify the raw body, persist/deduplicate, enqueue processing, and acknowledge.
7. Implement a fault-relay adapter and a versioned merchant verification contract. Ship a bundled reference merchant service so the full real workflow can be demonstrated locally.
8. Convert event streaming into a real long-lived SSE feed backed by appended database events, with reconnect and `Last-Event-ID` support.
9. Add protected APIs for onboarding, connections, environments, runs, findings, reports, notifications, settings, and audit history.
10. Add structured logs, OpenTelemetry traces, Prometheus metrics, health/readiness probes, rate limits, SSRF protection, redaction, and retention jobs.

Redpanda and ClickHouse remain optional scale adapters, not MVP dependencies. PostgreSQL plus a transactional outbox is the reliable MVP path.

## Product information architecture

| Route | Real user outcome |
| --- | --- |
| `/` | Understand the problem, interact with a truthful preview, enter product |
| `/login` | Authenticate |
| `/onboarding` | Create project and connect Sandbox/target |
| `/dashboard` | Live readiness, connection health, recent runs, findings, and actions |
| `/scenarios` | Browse, configure, validate, and launch scenarios |
| `/runs` | Filter/search persisted runs and compare results |
| `/runs/[id]` | Live topology, event stream, state diff, assertions, logs, and evidence |
| `/findings` | Triage, assign, resolve, and rerun failed invariants |
| `/reports/[id]` | Shareable/printable immutable verification report |
| `/connections` | Manage Stripe Sandbox, webhook, and verification target |
| `/environments` | Separate local, sandbox, and staging configurations |
| `/notifications` | Run completion and failure notifications |
| `/settings` | Project, retention, security, team, and audit settings |
| `/demo` | Guided seeded tour plus an explicit switch to a real connected run |

Every sidebar item becomes a route. Every visible action must navigate, mutate real state, open a functional control, or be removed. There will be no placeholder screens and no inert `View all`, `Inspect evidence`, account, notification, workspace, environment, or command-palette controls.

## UI and motion plan

### Marketing surface

- Preserve Optical Ledger, but rebuild the page as a scroll-directed forensic story.
- Pin the event braid while Browser, API, Stripe, Webhook, Worker, and Database chapters move through it.
- Use mask/clip reveals, depth parallax, stateful line drawing, kinetic but readable typography, and fault-to-convergence color transitions.
- Add magnetic primary actions, meaningful cursor proximity responses, animated architecture transitions, and a live run preview driven from real run-shaped data.
- Use GSAP ScrollTrigger for chapter choreography and Lenis only if it remains accessible and does not degrade keyboard/native scrolling.
- Keep content visible without JavaScript and provide reduced-motion and reduced-data alternatives.

### Product surface

- Motion communicates state: optimistic action feedback, skeleton-to-data transitions, event insertion, panel resizing, route continuity, command palette, toasts, and run completion.
- Keep routine transitions within 150–250 ms; reserve longer choreography for the marketing narrative and guided demo.
- Give every control default, hover, focus, active, disabled, loading, success, and error states.
- Make dense evidence easy to scan: resizable panels, sticky event filters, keyboard navigation, deep links, copy IDs, inspect raw/redacted payload, and export report.

### Visual quality gate

- Desktop, tablet, and mobile screenshot review at every completed route.
- WCAG 2.2 AA, keyboard-only flows, screen-reader names, reduced motion, reduced data, and high-contrast checks.
- No scroll jank: target smooth 60 fps motion, avoid layout-property animation, lazy-load heavy effects, and enforce bundle/performance budgets.

## Credential and access requirements

### Required for real local Stripe execution

- `STRIPE_SECRET_KEY`: an `sk_test_...` Sandbox secret, or preferably an `rk_test_...` restricted Sandbox key with only required PaymentIntent, Customer, Event, Refund, Billing, and Test Clock permissions.
- `NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY`: the matching `pk_test_...` key for a real Stripe Elements/Payment Element reference checkout.
- `STRIPE_WEBHOOK_SECRET`: `whsec_...` emitted by `stripe listen` locally or copied from the configured Sandbox webhook endpoint when hosted.

The Stripe CLI is not currently installed on this Mac. It is needed for the easiest local webhook workflow.

### Generated locally; no external account required

- `PARITYLAB_ENCRYPTION_KEY`: 32-byte key used to encrypt connection secrets.
- `AUTH_SECRET`: session-signing secret.
- `PARITYLAB_SIGNING_SECRET`: signs ParityLab-to-target verification requests.
- `DATABASE_URL`: provided by the local PostgreSQL Compose service.

### Needed only for hosted deployment

- Hosted PostgreSQL connection string.
- Authentication provider client ID/secret if OAuth login is enabled.
- Vercel access for the Next.js web deployment.
- Fly.io, Render, Railway, or equivalent access for the long-running Go API and worker.
- Optional error tracking/telemetry credentials.

Secrets must be entered into local `.env.local`/secret-manager UI, never pasted into chat, committed, exposed to browser bundles, or logged. Live Stripe keys are never accepted by v1.

## Implementation order and green gates

### Phase 0 — contracts and local environment

- Freeze the real run state machine, target verification contract, API schemas, threat model, and route map.
- Add secret-safe environment validation and install/configure Stripe CLI.
- Green gate: config tests, OpenAPI checks, and live-key rejection tests.

### Phase 1 — durable data foundation

- Wire migrations, PostgreSQL repositories, idempotency, webhook ledger, outbox, audit, and transactional tests.
- Green gate: restart persistence, concurrency, rollback, duplicate, migration up/down, and fresh-volume tests.

### Phase 2 — real Stripe Sandbox vertical slice

- Connect/validate Sandbox, create a real test PaymentIntent, receive a real signed webhook, persist it, grade one duplicate-delivery invariant, and expose its report.
- Green gate: one end-to-end real Sandbox run with evidence and zero seeded substitutions.

### Phase 3 — durable orchestration and scenario library

- Add worker leases/retries, fault relay, bundled merchant target, six remaining scenarios, Test Clock flow, and state probes.
- Green gate: process-restart recovery, idempotent replay, disorder, timeout, tamper, refund, and subscription tests.

### Phase 4 — live product UI

- Replace fixture-based overview/simulation state with APIs and SSE.
- Build every product route and wire every action, loading/empty/error state, command, notification, and export.
- Green gate: Playwright happy/error paths prove each route and every visible control.

### Phase 5 — premium marketing and motion

- Implement the pinned forensic narrative, scroll transitions, advanced event-braid choreography, hover physics, route/panel transitions, responsive adaptations, and motion fallbacks.
- Green gate: screenshot review, accessibility, reduced motion/data, animation performance, Core Web Vitals, and bundle budgets.

### Phase 6 — deploy and harden

- Deploy web, API, worker, and PostgreSQL; register HTTPS webhook; apply secrets; migrate; add monitoring, backups, rate limits, incident procedures, and CI/CD promotion.
- Green gate: fresh hosted setup, real Sandbox E2E, recovery drill, security scan, load test, and two consecutive clean CI runs.

## MVP completion definition

ParityLab is an actual MVP only when all of the following are true:

- A new user can connect a Stripe Sandbox and complete onboarding without editing source code.
- A real Stripe test object and signed event participate in a persisted verification run.
- Database restart does not erase runs, events, webhook deduplication, findings, or reports.
- The UI never silently presents fixture data as live data.
- Every route and visible control is functional and tested.
- A run survives worker/API restart according to its documented state machine.
- Reports are reproducible from stored evidence and clearly identify seeded versus real runs.
- Live Stripe keys/events are rejected, secrets are encrypted/redacted, and security tests pass.
- Marketing and product motion meet accessibility and performance gates.
- The deployed product can be demonstrated from sign-in through a real report.

## Proposed deployment

- Next.js web: Vercel.
- Go API and worker: a long-running container platform such as Fly.io or Render.
- PostgreSQL: managed PostgreSQL with automated backups.
- Stripe: dedicated Sandbox, restricted keys, and HTTPS webhook endpoint.
- Optional analytics/observability: OpenTelemetry plus the existing Prometheus/Tempo/Grafana profile; Redpanda/ClickHouse only after the PostgreSQL MVP is proven.

This split keeps the web globally fast while preserving long-lived SSE, webhook processing, and durable workers on infrastructure designed for them.
