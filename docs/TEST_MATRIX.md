# Test matrix

| Capability | Automated coverage | Current result |
| --- | --- | --- |
| Deterministic simulation | Node test runner: fixture order, divergence/convergence, progress bounds | 3/3 pass |
| Go domain engine | Seeded overview, validation, replay/conflict, fault timelines, concurrent dedupe | Pass in Go 1.26 container |
| HTTP API | Health, overview, scenarios, runs, JSON/SSE events, reports, CORS, body limits, request IDs | Pass |
| Webhook security | Valid/stale/tampered/live payloads, raw-body HMAC, redaction, event dedupe | Unit and live contract pass |
| Marketing narrative | Entry CTAs, duplicate injection, architecture keyboard controls | Chromium and mobile Chromium pass |
| Guided simulation | Story/Explore controls, timeline, evidence, sandbox label, real API run ID | Chromium and mobile Chromium pass |
| Dashboard | Readiness, engine connectivity, command surface, responsive overflow | Chromium and mobile Chromium pass |
| Accessibility | axe serious/critical, keyboard navigation, reduced motion | Pass on all three routes at both viewports |
| Production build | Next.js static generation and Go compile/vet | Pass |
| Dependency security | Secret-pattern scan and `pnpm audit --prod` | No known npm vulnerabilities |
| Infrastructure config | Compose render, Grafana JSON, shell/YAML/config checks | Pass |
| WebKit | CI matrix configured | Not run locally |
| Load/resilience | k6 API and duplicate webhook burst scripts | Authored, not run locally |
| Real Stripe Sandbox | Credential-gated adapter path | Not run; credentials absent |
