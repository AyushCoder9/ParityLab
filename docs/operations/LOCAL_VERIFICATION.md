# Local verification

## Fast loop

```bash
tests/scripts/verify-config.sh
tests/scripts/smoke-api.sh
```

`smoke-api.sh` expects the API on `http://localhost:8080` and creates one deterministic run. Override `PARITYLAB_API_URL` if needed.

## Browser and accessibility

```bash
cd tests/e2e
pnpm install --frozen-lockfile
pnpm exec playwright install chromium
pnpm test
```

Set `PARITYLAB_E2E_EXTERNAL_SERVERS=1` when web and API servers are already running. Set `PARITYLAB_WEB_URL` and `PARITYLAB_API_URL` for non-default ports.

## Performance

```bash
k6 run tests/k6/api-smoke.js
K6_BURST_EVENTS=10000 k6 run tests/k6/webhook-burst.js
```

The burst test requires a local seeded webhook token and signing secret. It refuses to run against non-loopback URLs unless `PARITYLAB_ALLOW_REMOTE_LOAD=1` is explicitly set.

## Security audit

```bash
tests/scripts/security-audit.sh
```

The script uses installed tools and fails on missing required project checks. CI additionally runs CodeQL, Gitleaks, Trivy, dependency review, and SBOM generation.

## Fresh-volume green run

```bash
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/green-run.sh
PARITYLAB_CONFIRM_FRESH=1 tests/scripts/green-run.sh
```

Run twice. The script deletes only the named Compose project volumes after resolving them through Compose. Never run it while local data must be preserved.
