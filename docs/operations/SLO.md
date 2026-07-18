# Service level objectives

These are product targets for a production-shaped deployment. Local and CI runs exercise the same indicators with smaller windows.

## Indicators and objectives

| Journey | SLI | Objective | Window |
| --- | --- | --- | --- |
| Webhook acceptance | Valid requests durably persisted and acknowledged / valid requests | 99.95% | Rolling 30 days |
| Webhook latency | Server duration before acknowledgement | p95 < 250 ms; p99 < 750 ms | Rolling 1 hour and 30 days |
| Run execution | Runs reaching a terminal result within five minutes / accepted runs | 99.9% | Rolling 30 days |
| Live updates | Persisted events visible to connected client | p95 < 2 s | Rolling 1 hour |
| Read API | Successful non-user-error requests / eligible requests | 99.9% | Rolling 30 days |
| Data correctness | Events with exactly one normalized effect and converged state / persisted events | 100%; any breach is an incident | Continuous |

Client cancellations, intentional fault-injection responses, invalid signatures, and rate-limited abusive traffic are excluded from availability denominators but remain observable.

## Error budgets

- 99.95% allows 21m 54s of equivalent unavailability in 30 days.
- 99.9% allows 43m 49s in 30 days.
- Correctness has no spendable budget: duplicate side effects, cross-tenant disclosure, lost accepted events, or live-mode processing stop releases immediately.

## Multi-window alert policy

| Severity | Condition | Action |
| --- | --- | --- |
| Page | 14.4x burn over 1h and 6x over 6h, or any correctness/security breach | Acknowledge in 10 minutes; begin mitigation |
| Ticket | 3x burn over 6h and 1x over 3d | Owner investigates during business hours |
| Warning | Queue age > 60s for 10m, dead-letter growth, storage > 80%, or SSE p95 > 2s | Inspect before it becomes user visible |

## Required metric dimensions

Bound cardinality. Keep `service`, `route_template`, `method`, `status_class`, `scenario`, `result`, and `environment`. Never use event ID, request ID, organization ID, URL, email, or Stripe object ID as metric labels.

Every request returns a request ID and propagates trace context. Logs may include tenant-safe internal IDs only after redaction.

## Load gates

- No loss and exact deduplication in a 10,000-delivery burst.
- Webhook p95 remains below 250 ms in the agreed local k6 profile.
- Dashboard LCP < 2.5s, CLS < 0.1, INP < 200ms on the agreed mid-range profile.
- No measured marketing-scroll long task > 50ms; renderer reduces fidelity below 50fps.

Update this document and the Grafana dashboard in the same change whenever an SLI name or threshold changes.
