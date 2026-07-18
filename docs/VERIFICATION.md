# Verification ledger

Date: 2026-07-18

## Environment

- macOS host, Node.js 26, pnpm 10.13.1.
- Go was not installed on the host; all Go checks used the official `golang:1.26-alpine` image.
- Web production server: `http://127.0.0.1:3100`.
- Isolated API container: `http://127.0.0.1:18080` because another user process already owned port 8080.

## Final commands

```bash
NEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:18080 pnpm build
pnpm lint
pnpm typecheck
pnpm test
pnpm audit --prod
docker compose -f infra/compose.yaml config -q
bash tests/scripts/verify-config.sh
```

Go checks were executed through the pinned container with the repository mounted at `/src`:

```bash
go test ./...
go vet ./...
go build ./...
```

Integrated browser gate:

```bash
PARITYLAB_WEB_URL=http://127.0.0.1:3100 \
PARITYLAB_API_URL=http://127.0.0.1:18080 \
PARITYLAB_E2E_EXTERNAL_SERVERS=1 \
pnpm --filter @paritylab/e2e exec playwright test \
  --project=chromium --project=mobile-chrome
```

Live webhook contract:

```bash
PARITYLAB_CONTRACT_API_URL=http://host.docker.internal:18080 \
go test -v ./tests/contracts
```

## Repair loop

1. Initial UI build and deterministic tests passed.
2. First E2E run passed 21/28 and found invalid ARIA labels, incomplete table roles, an unlabeled SVG, and desktop-only mobile assertions.
3. Product semantics and viewport-aware assertions were corrected; the next run passed 27/28.
4. The compact mobile brand semantic was repaired; the next run passed 28/28.
5. Dependency audit found high-severity advisories in the original Next.js and Playwright pins.
6. Next.js, React, Playwright, Axe, and PostCSS were upgraded to patched current releases; audit became clean and 28/28 E2E remained green.
7. The UI was connected to the Go engine through the shared contract package; integrated assertions prove a real API-created run ID and engine-online state.
8. Two consecutive final integrated browser runs passed 28/28 on the exact connected product tree.

## Deliberately not claimed

- No real Stripe key was available, so Stripe Sandbox network calls were not executed.
- WebKit and k6 were not run locally.
- The complete Compose dependency stack was not started against this shared Docker host.
