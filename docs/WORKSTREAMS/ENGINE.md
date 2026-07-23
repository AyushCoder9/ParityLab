# Engine workstream

Status: Phase 1 persistence, the credential-gated Phase 2 Stripe Sandbox PaymentIntent slice, the smallest green Phase 3 durable worker/reference-merchant slice, authentication/tenant resources, webhook correlation, and resumable durable SSE are implemented.

## Delivered

- Hexagonal `engine.Repository` persistence port with a credential-free, concurrency-safe in-memory adapter and a PostgreSQL 18 adapter backed by `pgx`.
- `DATABASE_URL` composition: unset uses the deterministic memory adapter; set connects, pings, applies checksum-locked migrations, and fails startup before listening if persistence is not ready.
- Atomic PostgreSQL run creation: run, ordered events, immutable report snapshot/hash, hashed idempotency record, and redacted transactional outbox message commit together.
- Durable reads for run detail, run list, event evidence, reports, overview values, idempotent replay, and webhook deduplication.
- `GET /v1/runs` list envelope for the live product UI; existing endpoints and deterministic seeded behavior remain compatible.
- Webhook ledger stores only event/type and SHA-256 body/endpoint hashes. The same event ID with changed signed content is rejected instead of being silently treated as a duplicate.
- Stable seeded IDs (`run_000001` through `run_000003`) and a database sequence floor ensure real/user-created runs start at `run_000004` without reseeding on restart.
- Reversible migrations for full run/event snapshots, immutable reports, users/memberships, encrypted Stripe connection and target secret columns, scenario configurations, worker leases, assertions, findings, notifications, audit records, and outbox claim metadata.
- Sandbox startup validation rejects `sk_live_`, `rk_live_`, and `pk_live_`; webhook ingress still rejects `livemode: true` before persistence.
- Graceful SIGINT/SIGTERM HTTP shutdown and repository pool close, with database credentials excluded from logs.

## Persistence and migration invariants

- Migration files remain explicitly wrapped in `BEGIN`/`COMMIT`; the migrator validates and strips those wrappers, then executes the body and checksum record in one pgx transaction under a session advisory lock.
- An already-applied migration whose bytes change is rejected by checksum rather than reapplied.
- Idempotency keys are stored only as SHA-256 hashes. Request bodies are stored only as SHA-256 hashes; replay responses contain the already-public run representation.
- Monetary scenarios describe and compare integer minor units plus ISO currency. No floating-point money fields were introduced.
- PostgreSQL outbox payloads contain only run ID, scenario ID, status, and the fixed sandbox environment—never keys, webhook bodies, or personal data.

## Verification evidence

- `docker run ... golang:1.26-alpine go test ./...` — passed after adding `pgx/v5` and formatting the backend.
- Live PostgreSQL 18 restart contract on host port `55432` — passed: API logged `persistence=postgres`; `run_000004` survived an API restart; the same idempotency key replayed the same run and returned `Idempotent-Replayed: true`; webhook `evt_root_restart` changed from `duplicate:false` before restart to `duplicate:true` after restart.
- Existing signed webhook, invalid-signature, CORS, Stripe-shaped errors, deterministic scenarios, and SSE unit contracts remain in `services/api/internal/httpapi` and `services/api/internal/engine`.
- Environment-gated adapter test: `TEST_DATABASE_URL=... go test ./services/api/internal/postgres -run TestRepositoryPersistsAcrossRestart -count=1 -v` exercises persisted run/events/report, replay, and webhook dedup across two repository instances.

## Runtime contract

```bash
DATABASE_URL='postgres://.../paritylab?sslmode=disable' \
PARITYLAB_MIGRATIONS_DIR='db/migrations' \
go run ./services/api/cmd/paritylab
```

The process does not listen until PostgreSQL is reachable and every migration is verified/applied. Therefore `GET /healthz` returning 200 after startup is the current readiness signal. If `DATABASE_URL` is omitted, ParityLab intentionally runs the deterministic in-memory demo.

## Deferred at the Phase 1 checkpoint

- Webhook domain processing beyond its durable ingress job remains a later Phase 3 expansion.
- The outbox is transactionally populated but no publisher/lease loop runs yet.
- Authentication and tenant authorization were deferred at that checkpoint and are now delivered in the authentication/tenant-resource slice below.
- Long-lived database-backed SSE was not part of the Phase 1 checkpoint. That limitation is superseded by the resumable event-streaming slice below.

## Phase 2 Stripe Sandbox vertical slice

### Delivered

- Official `github.com/stripe/stripe-go/v86` SDK at `v86.1.1`, using the current `stripe.Client` service API rather than the deprecated global client pattern.
- `POST /v1/connections/stripe/validate` validates `sk_test_` or `rk_test_` server-side through `GET /v1/account`, encrypts the key with AES-256-GCM, and returns only `{id,stripe_account_id,sandbox_name,status,created_at}`.
- PostgreSQL and memory connection stores. PostgreSQL persists only authenticated ciphertext plus a key-version marker; plaintext keys are never returned or logged.
- `POST /v1/stripe/payment-intents` requires `Idempotency-Key` and accepts `{connection_id,amount_minor,currency}`. Amount is an integer minor-unit value and currency must be a lowercase three-letter code.
- The PaymentIntent request carries stable non-sensitive `paritylab_correlation_id` and `paritylab_scenario_id` metadata. The external Stripe idempotency key is a SHA-256-derived stable token, never the raw client key.
- A successful Stripe response is graded through the duplicate-delivery invariant and atomically persisted as the same run/events/report/outbox representation used by the rest of ParityLab. The report includes the real `pi_` identifier and an exact minor-unit assertion.
- Repository idempotency preflight returns completed replays before decryption or network I/O and rejects changed request parameters locally. Concurrent misses still reach Stripe with byte-equivalent parameters and the same Stripe idempotency key, then converge through the repository transaction.
- `PARITYLAB_ENCRYPTION_KEY` must be a base64-encoded 32-byte key. Missing encryption configuration returns a safe 503; it never panics or falls back to plaintext storage.
- `STRIPE_API_BASE` is accepted only when `PARITYLAB_ALLOW_STRIPE_MOCK=true`; otherwise startup rejects the override. This exists for deterministic black-box tests and is not enabled by default.

### Exact verification

- `docker run --rm ... golang:1.26-alpine sh -c 'gofmt -w services/api && go test ./services/api/...'` — exit 0; `cmd/paritylab`, `engine`, `httpapi`, `postgres`, `secrets`, and `stripeadapter` passed.
- `docker run --rm ... golang:1.26-alpine sh -c 'go vet ./services/api/... && go build ./services/api/...'` — exit 0.
- Official SDK adapter tests use `httptest.Server` and assert authorization, `/v1/account`, form-encoded integer amount/currency, correlation metadata, and Stripe idempotency headers without external credentials.
- Engine tests prove exact replay returns the original persisted run without a second gateway call, changed parameters return a local 409 without reaching Stripe, the report stores the `pi_` evidence, and missing encryption configuration returns 503.
- HTTP tests prove both public endpoints, response redaction, direct persisted Run response, and `Idempotent-Replayed: true` behavior.

### Still credential-gated or deferred

- No real Stripe request was executed because no Sandbox secret/restricted key was provided. The adapter is production code, while CI/local tests use deterministic HTTP mocks.
- This slice creates an unconfirmed Sandbox PaymentIntent and persists the duplicate-delivery evidence model. A real signed Stripe webhook is still accepted by the durable ingress, but end-to-end correlation from that delivery into an asynchronous worker is Phase 3.
- Authenticated connections are now attached to the current project and cannot be used across tenants. The unauthenticated seeded/demo boundary remains separate for the public tour.
- Encryption supports version metadata but automated key rotation/re-encryption is deferred.

## Phase 3 durable worker and reference merchant

### Delivered

- Separate production worker entrypoint at `services/api/cmd/worker`. It requires `DATABASE_URL` and a minimum-16-byte `PARITYLAB_SIGNING_SECRET`, applies migrations, runs until SIGINT/SIGTERM, and closes PostgreSQL cleanly.
- PostgreSQL outbox claims use `FOR UPDATE SKIP LOCKED`, due-time ordering, named owners, attempt counters, expiring leases, heartbeat renewal, exponential retry scheduling, terminal failure timestamps, and safe error codes rather than exception text.
- Memory repository implements the same outbox contract for credential-free deterministic tests, including expired-lease recovery.
- Stripe-backed PaymentIntent runs enqueue `verification.run.queued` in the same transaction as the run, events, report, and idempotency response.
- First-seen signed Stripe webhooks now atomically persist the hashed receipt and enqueue one `stripe.webhook.received` outbox record before acknowledging. Duplicate deliveries return the existing duplicate response and enqueue nothing.
- Versioned `paritylab.verification.v1` request contract with HMAC-SHA256 authentication and controlled `none`, `duplicate`, `reorder`, `timeout`, and `tamper` relay behavior.
- Bundled reference merchant verifies the version/signature and applies effects through a durable `reference_merchant_effects` table. Effect idempotency and monotonic sequence state therefore survive worker restarts.
- The smallest completed worker vertical slice is: persisted Stripe run → outbox claim → duplicate delivery through the signed relay → exactly one durable reference-merchant effect → idempotent assertion appended to the persisted report and normalized assertions table → outbox completion.
- Migrations `000004_outbox_leases` and `000005_reference_merchant` are reversible and add the terminal-failure/claim index plus durable merchant effect state.

### Runtime

```bash
DATABASE_URL='postgres://.../paritylab?sslmode=disable' \
PARITYLAB_MIGRATIONS_DIR='db/migrations' \
PARITYLAB_SIGNING_SECRET='minimum-16-byte-secret' \
go run ./services/api/cmd/worker
```

The API and worker are intentionally separate processes. The API remains latency-focused and only commits webhook/run jobs; the worker owns relay execution and report enrichment.

### Verification

- `go test ./services/api/...` passes the API, worker command build, repositories, verification fault relay, durable reference merchant, and worker packages.
- Unit tests prove all five controlled fault modes; tampering produces zero effects, and duplicate/reorder produce one effect with duplicate evidence.
- Worker tests prove a queued Stripe run is claimed, processed, and receives `assert_reference_merchant_exactly_once`; expired leases are reclaimed by a different owner with an incremented attempt.
- Webhook tests prove first delivery enqueues exactly one `stripe.webhook.received` message while exact replay enqueues none.
- Environment-gated PostgreSQL worker integration is available through `TEST_DATABASE_URL`: it migrates, creates a Stripe-shaped queued run without external credentials, executes the worker, and reads the durable assertion from the report.

### Webhook correlation expansion

- Migration `000007_webhook_correlation` adds the sanitized webhook projection, processing state, correlation evidence, and uniqueness constraints required for durable replay-safe consumption.
- Signed ingress stores only the event creation time, event/object identifiers, allowlisted status, exact `paritylab_correlation_id`, event type, and body hash. Raw webhook bodies and arbitrary metadata are not persisted.
- The worker claims both verification and webhook topics. A supported PaymentIntent event is matched only by exact object ID plus a non-empty exact correlation ID, and the tenant is derived exclusively from the already-owned run.
- A successful match atomically creates one API-visible run event, one status-neutral report assertion, one normalized assertion, one webhook-evidence row, and the terminal processing state. Retrying after a crash or worker restart cannot duplicate those effects.
- Unsupported event types become terminal `ignored`; missing, unknown, mismatched, or ambiguous correlations become terminal `unmatched`. Malformed internal payloads become permanent failures, while transient repository failures retain the outbox retry contract.
- Verification jobs retain their previous behavior and unknown topics are not falsely published.

### Resumable durable event streaming

- `EventsAfter`, tenant-scoped `EventsAfterForProject`, and `PublicEventsAfter` return bounded ordered event batches plus an atomic high-water view from both PostgreSQL and memory adapters. PostgreSQL filters by sequence instead of rereading the complete ledger on every poll.
- `GET /v1/runs/{id}/events` keeps JSON behavior by default and becomes a long-lived SSE response only for `Accept: text/event-stream`.
- SSE strictly accepts one canonical non-negative decimal `Last-Event-ID`. Malformed, duplicate, overflow, leading-zero, and ahead-of-ledger cursors receive a Stripe-shaped 400 before stream headers are committed.
- Stable sequence IDs resume at `sequence > Last-Event-ID`; 100-event batching prevents unbounded response work. The stream emits `retry: 2000`, one truthful terminal `run.complete` snapshot, 15-second heartbeat comments, and later durable evidence without closing after the initial replay.
- Repository failures after headers produce only a sanitized `stream.error` frame and close so native clients reconnect with their last acknowledged ID. Request cancellation stops both tickers and the handler.
- The API retains its 15-second global write timeout for ordinary traffic. Each SSE write/flush refreshes a 10-second deadline through Go's `ResponseController`, preventing both absolute stream termination and indefinitely blocked slow clients.
- Authenticated streams derive the project from the opaque session. Anonymous access remains limited to public seeded runs; public or foreign-tenant IDs return 404.

### Remaining Phase 3 gaps

- The reference merchant is a bundled contract adapter backed by ParityLab PostgreSQL, not yet a separately deployable example merchant HTTP service.
- Persisted scenario runs now feed the verification relay through `run.persisted`, and duplicate, reorder, timeout, and tamper modes are selected from the stored run fault. The remaining gap is real Stripe refund and subscription/Test Clock executors plus broader production ops.
- Report enrichment is idempotent but mutates the preliminary report snapshot. A later state-machine slice must keep runs `running` until grading and emit the immutable final report only once.
- Worker metrics, OpenTelemetry spans, administrative dead-letter replay, and per-project concurrency/rate limits remain to be added.

## Authentication, tenancy, and persisted resources

### Delivered security model

- `POST /v1/auth/register`, `POST /v1/auth/login`, `POST /v1/auth/logout`, and `GET /v1/session` provide real account/session behavior. Registration is one transaction across the user, organization, owner membership, project, default environments, session, and audit event.
- Passwords use Argon2id. The normalized email is encrypted with the existing AES-256-GCM key and located using a keyed blind index; neither password nor plaintext email is persisted.
- Session tokens are generated from a cryptographic random source, returned only in an opaque cookie, and stored only as SHA-256 hashes. The cookie is 24-hour, `HttpOnly`, `SameSite=Lax`, and `Secure` by default. Logout revokes the database session; expired or revoked tokens fail closed.
- The explicit insecure-cookie override is accepted only for loopback development origins. Credentialed CORS is limited to the configured web origin, and cookie-authenticated mutations reject a missing/mismatched Origin as CSRF.
- Login returns the same invalid-credential error for known and unknown users, performs a dummy Argon2 verification on an unknown email, and applies a bounded in-process failure limiter keyed by client plus blind account index with a `Retry-After` response.
- Migration `000006_auth_tenancy` adds encrypted user/password fields, project retention, revocable hashed sessions, and per-project local/sandbox/staging environments with a single selected default.

### Delivered tenant/resource model

- Engine and Stripe repository ports now have project-scoped run, event, report, idempotency, connection, and PaymentIntent operations for both PostgreSQL and memory adapters.
- Tenant-created runs persist their project relationship, findings, completion notification, audit entry, and outbox work in the existing transaction. Public seeded/demo records remain `project_id IS NULL` and are never returned as tenant-owned records.
- `GET/PATCH /v1/settings/project`, `GET /v1/environments`, environment select, finding list/resolve/reopen, notification list/read/read-all, and sanitized connection list are protected, durable APIs.
- All resource lookups include the authenticated project. A valid identifier from another tenant returns 404, including Stripe connection use, environment selection, finding mutation, and notification mutation.
- Environment switching clears and sets the single selected environment within one transaction. Protected resource mutations also write tenant audit events.

### Authoritative integration evidence

`PARITYLAB_CONFIRM_FRESH=1 tests/scripts/auth-resource-restart.sh` passed against its dedicated Compose project. The harness starts from a fresh database and strict Stripe mock, verifies default Secure and loopback cookie modes, registration redaction, protected Stripe calls, known/unknown login parity, throttling, durable settings/environment/finding/notification mutations, tenant isolation, CSRF, expired sessions, API-restart persistence, logout revocation, and API-log secret absence. Its cleanup is restricted to the named auth test project and volumes.

Focused auth, HTTP route, repository, environment-switch, and tenant-memory tests accompany that black-box harness. The separate authenticated browser gate also passed Chromium 17/17 and WebKit 17/17 against the fresh integrated stack.

### Remaining identity/operations work

- The current registration creates one owner organization/project. Invitations, additional projects/project switching, password recovery, email verification, MFA/passkeys, and a user-facing session inventory are future slices.
- Login throttling is process-local; a multi-instance deployment needs a shared limiter and broader abuse controls.
- Encryption data carries key-version metadata, but automated email/Stripe-secret rotation and re-encryption are not implemented.
