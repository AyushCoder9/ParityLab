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
| Accessibility | axe serious/critical across marketing, demo, auth, onboarding, and all product routes; keyboard navigation and reduced motion | Mobile Chromium pass |
| Production build | Next.js static generation and Go compile/vet | Pass |
| Dependency security | Secret-pattern scan and `pnpm audit --prod` | No known npm vulnerabilities |
| Infrastructure config | Compose render, Grafana JSON, shell/YAML/config checks | Pass |
| WebKit | CI matrix configured | Not run locally |
| Load/resilience | k6 API and duplicate webhook burst scripts | Authored, not run locally |
| Real Stripe Sandbox | Credential-gated adapter path | Not run; credentials absent |
| Product route map | Playwright: all 11 static product routes, headings, placeholder rejection, real navigation links and mobile More menu | Mobile Chromium pass |
| Dynamic evidence routes | Playwright: live/seeded run and seeded report drill-down with explicit provenance | Mobile Chromium pass |
| Runtime truthfulness | Playwright request failure injection: explicit unavailable/seeded state, no fake live run ID | Mobile Chromium pass |
| Product control wiring | Playwright: notifications, account, overview actions, evidence, command navigation | Mobile Chromium pass |
| Product accessibility | axe serious/critical scan across marketing, demo, auth, onboarding, and all static product routes | Mobile Chromium pass |
| Product responsive layout | 390x844 overflow check across all static product routes | Mobile Chromium pass |
| Browser/local state boundary | Playwright: preview settings, findings, notifications, environments make no API mutation and disclose local scope | Chromium 7/7 state-boundary suite pass |
| Sanitized Stripe connection UI | Playwright mocked POST: secret sent once, never rendered/stored, sanitized account record only | Chromium pass |
| Stripe PaymentIntent UI wiring | Playwright mocked frozen route: UUID connection, 4200 minor units, usd, returned run navigation | Chromium pass |
| Stripe OpenAPI contract | Static executable gate: paths, operation IDs, write-only secret, sanitized response, amount/currency constraints | Pass |
| PostgreSQL migrations/readiness | Dedicated PostgreSQL 18 Compose contract; API health only after startup migration | Fresh Compose pass |
| Strict Stripe adapter contract | Local Stripe mock validates auth, account lookup, PaymentIntent form, correlation/scenario metadata | Pass |
| Restart persistence | Black-box API restart: encrypted connection, Stripe/deterministic runs, reports/events, idempotency, webhook dedupe | Pass; `run_000005` survived, exit 0 |
| Browser-to-Stripe-mock vertical | UI connection and 4200-USD run through live API, strict Stripe mock, PostgreSQL-backed ledger, and balanced report | Chromium 1/1 pass (opt-in) |
