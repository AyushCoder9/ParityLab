# ParityLab implementation plan

ParityLab continuously verifies Stripe integrations against duplicate delivery, event disorder, retries, endpoint faults, subscription-time changes, concurrency, and API drift.

## Product slices

1. Durable repository context and runnable monorepo.
2. Seeded reliability API and deterministic scenario runner.
3. Optical Ledger website, guided simulation, and authenticated dashboard.
4. Stripe Sandbox adapter, signed webhook ingress, idempotency, and persistence.
5. Browser, integration, resilience, security, accessibility, and performance verification.

## Frozen decisions

- Web: Next.js 16, React 19, strict TypeScript, and a native CSS token/component system.
- Engine: Go 1.26 with ports-and-adapters boundaries.
- Source of truth: PostgreSQL; streaming and analytics adapters remain optional locally.
- Visual direction: Optical Ledger. Marketing is cinematic; product UI is restrained.
- Completion: local seeded demo and clean automated verification. Real Stripe tests run when sandbox credentials exist.

## Public API

- `GET /healthz`
- `GET /v1/overview`
- `GET /v1/scenarios`
- `POST /v1/runs`
- `GET /v1/runs/{id}`
- `GET /v1/runs/{id}/events`
- `GET /v1/runs/{id}/report`
- `POST /hooks/stripe/{endpoint_token}`

Mutating requests accept `Idempotency-Key`. Errors use `{ "error": { "type", "code", "message", "param", "request_id" } }`.
