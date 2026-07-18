# Architecture

```text
Browser / Stripe Sandbox
          |
          v
  Go HTTP control plane
  - raw webhook verification
  - idempotency and run API
  - deterministic scenario state machine
          |
          +---- PostgreSQL + transactional outbox
          |
          +---- event stream adapter ---- analytics adapter
          |
          v
 Next.js product + SSE run viewer
```

The local default is deliberately self-contained: the API uses an in-memory repository with deterministic fixtures so the complete product runs without credentials. PostgreSQL, Redpanda, ClickHouse, and observability adapters are production-shaped infrastructure profiles rather than prerequisites for the first interaction.

## Invariants

- A run is immutable after reaching a terminal state.
- A scenario step is applied at most once for an idempotency key.
- Duplicate event delivery cannot duplicate a business mutation.
- Event order is not trusted; current resource state wins.
- Live Stripe data is rejected in v1.
- Pass/fail is deterministic; explanation layers cannot modify the score.
