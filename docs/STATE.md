# Project state

Updated: 2026-07-18

## Current milestone

The production-grade local v1 is implementation-complete and in final repository handoff. Marketing, guided simulation, dashboard, Go reliability engine, shared contracts, webhook ingress, infrastructure profiles, CI, security documentation, and automated verification are present.

## Completed product surface

- Premium Optical Ledger marketing route at `/`.
- Deterministic Story/Explore failure simulator at `/demo`.
- Responsive authenticated-style product dashboard at `/dashboard`.
- Live browser-to-Go integration: the simulator creates idempotent API runs and the dashboard reports engine connectivity.
- Five API scenarios, four visually guided scenarios, overview, run detail, JSON/SSE events, report, and signed webhook endpoints.
- Stripe-shaped request errors, request IDs, raw-body signature verification, timestamp tolerance, sandbox-only enforcement, duplicate suppression, redaction, CORS, and body limits.
- PostgreSQL 18 migration and optional Redpanda, ClickHouse, OpenTelemetry, Prometheus, Tempo, and Grafana profiles.
- Stripe App extension skeleton, OpenAPI/TypeScript contracts, CI, security scanning, SBOM, SLO, threat model, and incident runbook.

## Exact green evidence

- `NEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:18080 pnpm build` — pass on Next.js 16.2.10; `/`, `/demo`, `/dashboard`, and not-found prerendered.
- `pnpm lint` — pass.
- `pnpm typecheck` — pass for web and Playwright projects.
- `pnpm test` — pass, 3/3 deterministic simulation tests.
- Containerized `go test ./...` on `golang:1.26-alpine` — pass for engine, HTTP API, and contract packages.
- Containerized `go vet ./... && go build ./...` — exit 0.
- Live container contract — signed webhook deduplication and invalid-signature rejection both pass against the running API.
- Integrated Playwright Chromium + mobile Chromium — two consecutive final runs passed 28/28, including axe serious/critical scan, keyboard navigation, responsive overflow, API, idempotency, UI/API connectivity, and product behavior.
- `docker compose -f infra/compose.yaml config -q` and `tests/scripts/verify-config.sh` — pass.
- `pnpm audit --prod` — no known vulnerabilities after patched framework/tooling upgrades and PostCSS override.
- Desktop and mobile screenshots were inspected manually; no blocking layout defects found.

See `docs/VERIFICATION.md` for the complete command ledger and repair history.

## Honest limitations

- Real Stripe Sandbox execution is credential-gated and was not run because no test keys or webhook secret were provided. The deterministic sandbox path is fully functional without credentials.
- The local runtime adapter is intentionally in-memory. The PostgreSQL/outbox schema and infrastructure profile exist, but the API does not yet persist runs or publish outbox messages.
- WebKit is configured in CI but was not executed locally because only Chromium was installed.
- k6 scripts exist but k6 load tests were not executed locally.
- The full dependency Compose stack was configuration-validated, not booted through two destructive fresh-volume cycles on this shared Docker host.

## Last green implementation commit

`16e2b72` — `fix: turn hero whitespace into verification rail`

## Desktop mirror

Verified clean at `/Users/ayushkumarsingh/Desktop/PROJECTS/SideProjects/ParityLab`. The mirror is refreshed after each green handoff commit.

## Next optional expansions

1. Provide Stripe test credentials and run the real Sandbox adapter path.
2. Wire the PostgreSQL/outbox runtime adapter and execute the two fresh-volume Compose cycles.
3. Run WebKit and k6 locally, or allow the configured CI matrix to provide that evidence.
