# Detailed implementation plan

## Workstream A: product and premium UI

### Marketing

- Hero states the product claim and renders the event braid immediately after accessible copy.
- A fault-injection chapter demonstrates duplicate delivery and business-effect deduplication.
- Architecture, scenario coverage, evidence, and final CTA chapters use varied layouts rather than repeated cards.
- Motion is cinematic on first/rare interactions and restrained elsewhere.

### Guided simulation

- Story mode provides a recruiter-friendly guided run.
- Explore mode exposes scenarios, faults, playback, event timeline, findings, assertions, and state comparison.
- UI state is derived from the deterministic run model; it is not a prerecorded animation.

### Dashboard

- Overview prioritizes readiness, run action, event topology, critical finding, recent runs, distribution, and timeline.
- Stable navigation labels: Overview, Simulations, Events, Findings, Integrations, Environments, Alerts, Reports, Team/Billing, Settings.
- Keyboard, responsive, reduced-motion, reduced-data, loading, empty, error, and not-found states are required.

### Visual system

- Optical Ledger combines pure white, neutral obsidian, deep viridian, mint verification, and signal coral faults.
- Marketing uses the Etched Optics interpretation.
- Simulation uses the Split Spectrum interpretation.
- Dashboard uses the Instrument Panel interpretation.
- The event braid is the one signature extraordinary effect.

## Workstream B: reliability engine

### Domain

- Scenario, Run, Event, Assertion, Finding, Report, Overview, and state-snapshot models.
- Deterministic seeded data and timestamps where tests require reproducibility.
- Terminal run states are immutable.

### Application service

- List scenarios and overview.
- Create or retrieve a run by idempotency key.
- Produce ordered event timelines and deterministic evidence reports.
- Reject invalid scenario/fault combinations.

### HTTP transport

- Health, overview, scenarios, run creation/detail/events/report, and webhook ingress.
- Stripe-shaped error envelopes with request IDs.
- CORS limited to configured local/product origins.
- SSE event output with stable event IDs and terminal completion.

### Webhooks

- Verify HMAC over the untouched body.
- Enforce timestamp tolerance and constant-time comparison.
- Reject `livemode: true` and malformed payloads.
- Deduplicate by event identity and preserve order independence.
- Redact sensitive fields from logs/reports.

### Persistence and production adapters

- In-memory deterministic adapter is the required local path.
- PostgreSQL schema covers organizations, projects, connections, targets, runs, events, findings, audit log, and outbox.
- Redpanda and ClickHouse are production-shaped profiles and must not block the core local demo.

## Workstream C: verification and operations

- Go unit, HTTP, webhook, and contract tests.
- TypeScript deterministic simulation tests.
- Playwright marketing, product, responsive, API, idempotency, and axe checks.
- k6 API and duplicate-webhook burst scripts.
- Docker Compose validation and observability configuration.
- CI for builds, tests, secret scan, dependency scan, CodeQL, container scan, and SBOM.
- Threat model, SLOs, incident runbook, and fresh-green procedure.

## Integration order

1. Freeze public API and semantic UI contracts.
2. Land deterministic UI and engine independently.
3. Build and typecheck both lanes.
4. Run production UI and inspect screenshots at desktop/mobile.
5. Run Go 1.26 tests in a local toolchain or pinned container.
6. Start API and web together; run Playwright Chromium suite.
7. Repair mismatches and repeat targeted checks.
8. Run security/config validation.
9. Update `STATE.md` with exact evidence and limitations.
10. Mirror the complete Git repository to the selected Desktop path.

## Green definition

Green means every available required command passes twice from a clean state. Credential-gated Stripe checks are marked pending, never silently replaced with mocks. The seeded demo remains fully functional without those credentials.
