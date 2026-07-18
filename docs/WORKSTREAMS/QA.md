# Verification workstream

Status: implementation complete; full integration evidence pending

## Completed

- Local PostgreSQL, Redpanda, ClickHouse, OpenTelemetry, Prometheus, Tempo, and Grafana stack.
- Threat model, SLOs, incident runbook, and local verification guide.
- Playwright browser/accessibility contracts and k6 load profiles.
- CI, security scanning, SBOM, and local audit/smoke scripts.

## Required integration changes (root-owned)

- Confirm metric names in `infra/grafana/dashboards/paritylab-overview.json` when instrumentation lands.

## Verification evidence

- `pnpm --filter @paritylab/e2e typecheck` — passed.
- `tests/scripts/verify-config.sh` — passed; Compose rendered cleanly; Grafana JSON parsed.
- `sh -n tests/scripts/*.sh` — passed.
- `node --check tests/k6/*.js` — passed.
- GitHub workflow YAML parsed successfully.
- Go contract compilation was not available locally because this machine has no Go toolchain; CI provisions Go 1.26.

## Integration follow-up

- Run the complete E2E suite when Playwright browsers and Go 1.26 are available.
- Start the full dependency stack once to verify image availability and health checks on the target machine.
- Record two clean fresh-volume runs.
