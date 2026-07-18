# Handoff

## Resume

```bash
cd /Users/ayushkumarsingh/Documents/Codex/2026-07-18-here-is-the-stripe-internship-details/ParityLab
git status --short --branch
make verify
```

Read in this order:

1. `AGENTS.md`
2. `docs/PROJECT_BRIEF.md`
3. `docs/STATE.md`
4. `docs/IMPLEMENTATION_PLAN.md`
5. `docs/PLAN.md`
6. The active `docs/WORKSTREAMS/*.md` file
7. `docs/VERIFICATION.md`

These files contain the user requirements, approved decisions, implementation map, and exact current evidence. Do not reconstruct important context from chat history.

The active build is in the Documents/Codex path above. The final mirror is at `/Users/ayushkumarsingh/Desktop/PROJECTS/SideProjects/ParityLab` after handoff. If the two copies differ, prefer the Git copy whose `docs/STATE.md` names the latest green commit.

## Commands

```bash
make dev        # local web + API instructions
make test       # TypeScript and Go tests
make build      # production builds
make verify     # formatting, tests, and builds
make infra-up   # PostgreSQL and observability dependencies
make infra-down
```

## Environment

Copy `.env.example` to `.env.local`. The seeded demo requires no external credentials. Never commit secrets.

If the host has no Go installation, use `golang:1.26-alpine` as documented in `docs/VERIFICATION.md`. For a normal local run, use web port 3000 and API port 8080; the verification ledger used 3100/18080 only to avoid existing host processes.
