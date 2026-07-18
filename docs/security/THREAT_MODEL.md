# ParityLab threat model

Status: living document. Review after changes to authentication, webhook ingress, target probes, retention, or Stripe permissions.

## Security objectives

1. A tenant can observe and mutate only its own projects, runs, evidence, and connections.
2. Stripe credentials and webhook secrets never appear in logs, analytics, reports, browser payloads, or traces.
3. Only authentic, timely Stripe test-mode events enter the event pipeline.
4. Duplicate or reordered delivery cannot produce duplicate business effects.
5. User-controlled target URLs cannot reach local, link-local, cloud metadata, or private network services.
6. ParityLab cannot initiate a live-mode payment or accept a live-mode Stripe event.

## Trust boundaries and data flow

```text
Browser -> Web/BFF -> API -> PostgreSQL/outbox -> Redpanda -> workers -> ClickHouse
                       ^                                  |
Stripe test webhooks --|                                  +-> reports/UI stream
                       |
                       +-> approved HTTPS merchant target / Stripe sandbox API
```

- Browser-to-web and web-to-API requests cross an authentication and tenant-authorization boundary.
- Webhook ingress is unauthenticated at the network edge; an opaque endpoint token, raw-body signature verification, timestamp tolerance, and rate limiting establish trust.
- Stripe and merchant target calls cross an outbound-network boundary.
- Analytics and observability are lower-trust sinks: they receive identifiers and aggregates, never secrets or full payment payloads.

## Assets and classification

| Asset | Classification | Storage/handling |
| --- | --- | --- |
| Stripe restricted/test secret | Secret | Envelope encrypted; server only; redact everywhere |
| Webhook signing secret | Secret | Envelope encrypted; raw-body verifier only |
| OAuth refresh/access token | Secret | Envelope encrypted; least-privilege scopes |
| Raw Stripe event | Sensitive | Encrypted storage; bounded retention; field-level report allowlist |
| Customer/payment identifiers | Sensitive | Tokenized or truncated in UI/logs |
| Run findings and traces | Internal | Tenant scoped; redacted before export |
| Audit events | Internal/immutable | Append-only; actor, action, target, request ID |

## STRIDE analysis

| Threat | Attack example | Required control | Verification |
| --- | --- | --- | --- |
| Spoofing | Forged webhook or guessed endpoint token | Stripe signature over untouched body; constant-time compare; five-minute tolerance; opaque 128-bit token | Signed, stale, malformed, tampered fixtures |
| Spoofing | Stolen session crosses organizations | Short-lived session, secure cookies, membership check on every resource query | Tenant-isolation contract tests |
| Tampering | Event body mutated before verification | Capture request body once as bytes; verify before parsing; persist body hash | Raw-body signature tests |
| Tampering | Client changes run/project ID | Server derives organization from principal; scoped database query | IDOR negative tests |
| Repudiation | Operator replays or changes a connection | Append-only audit record with actor, request ID, timestamp, result | Audit trail integration test |
| Information disclosure | Secret or PII enters logs/traces | Structured allowlist logging; redaction middleware; no request-body logging | Canary-secret scan over test logs/reports |
| Information disclosure | Report URL guessed | Authorization on every fetch; opaque IDs are not authorization | Cross-tenant report test |
| Denial of service | Webhook flood or huge body | Edge/body-size limits; per-endpoint rate limit; fast durable ack; bounded queues | k6 burst and 413/429 tests |
| Denial of service | Poison message loops forever | Bounded retries, dead-letter stream, idempotent replay, operator alert | Chaos replay test |
| Elevation of privilege | Target probe reaches metadata service | HTTPS-only URL; DNS/IP validation before and after redirects; block private/link-local/loopback/reserved ranges; egress policy | SSRF table tests and DNS-rebinding test |
| Elevation of privilege | Development secret adapter enabled in production | Startup assertion on environment; OAuth only outside local/test | Production-config negative test |
| Safety violation | Live key/event used accidentally | Reject `sk_live_`, `rk_live_`, `pk_live_`; reject `livemode: true`; never expose live mode selector | Unit + API rejection tests |

## Abuse cases

### Duplicate-event amplification

An attacker repeatedly submits one valid captured test event. The ingress record has a unique Stripe event ID and account scope; consumers use idempotency keys and transactional checkpoints. Replays are observable but do not create extra findings or merchant mutations.

### Event-order manipulation

An attacker delays an older subscription event until after a newer state transition. Consumers do not treat arrival order as source-of-truth; they compare object version/time and hydrate current Stripe state when required.

### SSRF through target registration

A member registers `https://example.test` that resolves to a public IP, then changes DNS to `169.254.169.254`. Resolution is validated at registration and connection time, redirects are revalidated, the transport pins the approved IP for a request, and infrastructure egress blocks reserved ranges.

### Evidence exfiltration

A member shares a report link outside the organization. Reports require authentication and tenant authorization by default. Explicit exports are redacted, expire, and are recorded in the audit log.

## Minimum security headers

- `Content-Security-Policy` with nonces/hashes and restrictive `connect-src`.
- `Strict-Transport-Security` in production.
- `X-Content-Type-Options: nosniff`.
- `Referrer-Policy: strict-origin-when-cross-origin`.
- `Permissions-Policy` disabling unused sensors and APIs.
- Frame protection through CSP `frame-ancestors`; allow only explicitly supported Stripe App contexts.

## Secret and retention policy

- Store development credentials only in ignored local environment files; production uses a managed secret store.
- Rotate a connection immediately after suspected disclosure and make rotation auditable.
- Raw event bodies default to 30-day retention, findings to 90 days, aggregate metrics to 13 months, and audit events to one year. Organization policy may shorten these windows.
- Deletion is tenant scoped, asynchronous, idempotent, and verified across PostgreSQL, ClickHouse, object storage, and backups.

## Release security gate

- `go test ./...`, race/fuzz targets where supported, `govulncheck ./...`.
- Frozen-lockfile install, typecheck/lint/tests/build, production dependency audit.
- Gitleaks full-history scan and Trivy filesystem/configuration scan.
- Contract tests for signature verification, live-mode rejection, tenant isolation, SSRF, redaction, and safe error envelopes.
- No critical/high vulnerability without an owner, written risk acceptance, expiry date, and compensating control.
