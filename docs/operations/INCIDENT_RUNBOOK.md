# Incident runbook

## First ten minutes

1. Declare the incident, name an incident commander, and start a timestamped log.
2. Classify impact: acceptance, delayed processing, incorrect results, data loss, tenant isolation, secret exposure, or live-mode safety.
3. Freeze deployments and destructive replays. Preserve request IDs, trace IDs, queue offsets, database/outbox state, and relevant configuration hashes.
4. Mitigate before diagnosing deeply: disable the affected scenario/connection, shed optional traffic, scale consumers, or isolate a tenant.
5. For cross-tenant access, secret exposure, lost accepted events, duplicate business effects, or any live-mode signal, treat severity as critical and engage security immediately.

Do not delete a queue, truncate a table, rotate all secrets, or replay a dead-letter stream until the exact scope is resolved and the action has a rollback/recovery plan.

## Triage commands

```bash
docker compose -f infra/compose.yaml ps
docker compose -f infra/compose.yaml logs --since=15m otel-collector redpanda postgres
curl --fail --silent --show-error http://localhost:8080/healthz
curl --fail --silent --show-error http://localhost:9644/v1/status/ready
curl --fail --silent --show-error http://localhost:8123/ping
```

Open Grafana at `http://localhost:3001` and inspect webhook acknowledgement latency, run outcomes, queue age, consumer errors, and trace exemplars. Search logs by request/trace ID, not raw payload content.

## Failure playbooks

### Webhook acknowledgement errors or latency

- Confirm ingress can reach PostgreSQL and that transaction latency is healthy.
- Compare accepted request count with raw-event and outbox inserts.
- If persistence is healthy but downstream work is slow, keep accepting and scale/pause non-critical consumers.
- If persistence cannot be guaranteed, return retryable errors; never acknowledge an event that was not durably stored.

### Consumer lag or dead-letter growth

- Identify the first failing offset and error class.
- Confirm the message is safe to inspect through redacted metadata.
- Deploy an idempotent fix, test one quarantined fixture, then replay a bounded range while monitoring duplicate-effect counters.
- Record start/end offsets and counts in the incident log.

### Suspected duplicate effects

- Disable the mutating consumer while leaving ingress available.
- Compare event uniqueness key, consumer checkpoint, and resulting merchant mutation key.
- Do not bulk repair. Generate a tenant-scoped reconciliation plan and require review.
- A single confirmed duplicate effect is an SLO correctness breach.

### Cross-tenant access or report disclosure

- Disable the affected route/export and revoke active shares.
- Preserve authorization/audit logs; identify organizations, actors, fields, and time range.
- Engage security and legal notification process. Avoid putting sensitive details in general incident chat.

### Credential exposure

- Disable the connection and revoke/rotate only confirmed or plausibly affected credentials.
- Search redacted log indexes, traces, CI artifacts, and repository history for the canary/fingerprint—never paste the secret into a command line or ticket.
- Reauthorize with least privilege and validate the audit trail.

### Live-mode key or event detected

- Reject and quarantine metadata; do not persist the complete body beyond the secured evidence path.
- Disable the connection and stop any worker with live credentials.
- Verify that ParityLab made no live API mutations. Escalate as critical even if no charge occurred.

## Recovery and closure

1. Reconcile Stripe current state, raw ingress, outbox, stream offsets, normalized records, analytics, and merchant probe state.
2. Run the failing fixture plus duplicate, reorder, restart, and tenant-isolation tests.
3. Observe two clean processing windows before closing mitigation.
4. Write a blameless review with timeline, trigger, contributing conditions, user impact, detection gap, and durable actions.
5. Add a regression test and update this runbook, the threat model, or SLOs when assumptions changed.
