# ParityLab

Continuous verification for Stripe integrations. ParityLab injects realistic delivery and state faults, evaluates deterministic invariants, and explains whether the integration converged safely.

## Quick start

Requirements: Node.js 24+, pnpm 10+, and Go 1.26+.

```bash
pnpm install
pnpm dev
go run ./services/api/cmd/paritylab
```

Open `http://127.0.0.1:3000`. The seeded simulation works without Stripe credentials and automatically connects to the sandbox API when it is available at `http://127.0.0.1:8080`.

Product routes:

- `/` — cinematic product narrative and interactive fault demonstration.
- `/demo` — deterministic Story/Explore simulation with real API-created runs.
- `/dashboard` — responsive readiness, findings, topology, runs, and engine status.

See `docs/HANDOFF.md` for verification and infrastructure commands and `docs/VERIFICATION.md` for the exact green evidence.

For a complete description of the original user requirements and why this project was selected, read `docs/PROJECT_BRIEF.md`. New agents should start with `docs/NEW_CHAT.md`.
