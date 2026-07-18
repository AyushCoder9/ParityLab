# ParityLab Agent Contract

Read `docs/STATE.md`, `docs/HANDOFF.md`, and `docs/PLAN.md` before changing code.

## Ownership

- Root agent: contracts, integration, repository state, final verification.
- UI lane: `apps/web/**`, `packages/ui/**`.
- Engine lane: `services/**`, `db/**`.
- Verification lane: `tests/**`, `.github/**`, `infra/**`, test documentation.

Do not modify another lane's files without recording the reason in the relevant workstream file.

## Non-negotiables

- Sandbox/test mode only. Reject live Stripe keys and `livemode: true` events.
- Money is stored as integer minor units with an ISO currency.
- Mutations accept idempotency keys; webhook consumers are idempotent and order-independent.
- Never log credentials, webhook secrets, complete payment payloads, or personal data.
- UI must remain keyboard usable, WCAG 2.2 AA, responsive, and useful with reduced motion.
- No task is complete until its tests and production build pass.

## Handoff protocol

After an integration slice, update the appropriate `docs/WORKSTREAMS/*.md`. The root agent alone updates `docs/STATE.md`. Record exact commands and outcomes, not conversational history.

## Validation

```bash
make verify
```

Targeted commands are documented in `docs/HANDOFF.md`.
