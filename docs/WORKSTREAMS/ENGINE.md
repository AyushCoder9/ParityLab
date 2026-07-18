# Engine workstream

Status: complete for the local v1

## Delivered

- Deterministic five-scenario and three-seeded-run engine.
- Overview, scenario list, idempotent run creation/detail, JSON/SSE event timeline, evidence report, and webhook ingress.
- Stripe-shaped error envelope with request IDs.
- Exact replay and conflict behavior for `Idempotency-Key`.
- Raw-body HMAC verification, five-minute tolerance, constant-time comparison, livemode rejection, response redaction, and concurrent-safe event-ID deduplication.
- PostgreSQL 18 schema for projects, connections, targets, runs, events, findings, audit records, idempotency records, and outbox.
- Shared TypeScript and OpenAPI contracts.

## Verification

- `go test ./...` in `golang:1.26-alpine` — pass.
- `go vet ./... && go build ./...` in the same pinned image — exit 0.
- Live contract against the running service — signed duplicate delivery and invalid signature cases pass.
- Browser API and idempotency contracts — pass in desktop and mobile Chromium.

## Deferred production adapter

The v1 uses the in-memory deterministic adapter. Wiring PostgreSQL persistence and the outbox publisher is the next backend expansion; the schema and infrastructure are already present.
