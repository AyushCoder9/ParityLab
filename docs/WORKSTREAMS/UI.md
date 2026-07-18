# UI workstream

Status: complete for seeded vertical slice

## Delivered

- Optical Ledger tokens, reusable primitives, original SVG brand mark, code-native event braid, and application shells.
- Cinematic marketing route at `/` with interactive fault injection, architecture explorer, scenario coverage, evidence diff, and dashboard handoff.
- Guided deterministic simulation at `/demo` with Story/Explore modes, scenario selection, play/pause, scrubbing, playback speed, semantic event log, and expected-versus-observed evidence.
- Authenticated-style dashboard at `/dashboard` with stable Overview/Simulations/Events/Findings navigation, readiness, topology, latest finding, recent runs, activity, command palette, and seeded product states.
- Responsive layouts for compact mobile, tablet, and desktop; keyboard focus states; reduced-motion and reduced-data alternatives; SVG particle motion is omitted when reduced motion is requested.
- Deterministic scenario tests for ordering, progress clamping, divergence, and convergence.

## Verification

- `pnpm lint` — pass, TypeScript strict/no-emit.
- `pnpm test` — pass, 3/3 deterministic simulation tests.
- `pnpm build` — pass, Next.js 16 production build; `/`, `/demo`, `/dashboard`, and `/_not-found` statically prerendered.

## Handoff notes

- The UI intentionally uses no external image or font network dependency. CSS names Spline Sans and Martian Mono first and uses carefully matched system fallbacks when they are not installed.
- Seeded data is isolated in `apps/web/src/lib/simulation.ts`, ready to replace with API/SSE data without changing the interaction model.
- Live browser screenshot and cross-browser checks remain the integration/QA lane's final gate.
