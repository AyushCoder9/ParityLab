export type EventStatus = "passed" | "active" | "fault" | "waiting";

export type RunEvent = {
  id: string;
  at: number;
  source: "Browser" | "API" | "Stripe" | "Webhook" | "Worker" | "Database";
  label: string;
  detail: string;
  status: EventStatus;
};

export type Scenario = {
  slug: string;
  name: string;
  summary: string;
  faultAt: number;
  duration: number;
  evidence: string;
  expected: string;
  observed: string;
  events: RunEvent[];
};

const baseEvents: RunEvent[] = [
  { id: "ui_01J8Z2", at: 0, source: "Browser", label: "Checkout submitted", detail: "A customer confirms a $42.00 test payment.", status: "passed" },
  { id: "req_01J8Z3", at: 12, source: "API", label: "PaymentIntent created", detail: "Idempotency key checkout_842 accepted.", status: "passed" },
  { id: "pi_3Q9K8", at: 24, source: "Stripe", label: "Payment confirmed", detail: "payment_intent.succeeded produced.", status: "passed" },
  { id: "evt_1Q9R2", at: 40, source: "Webhook", label: "Event delivered", detail: "Signature verified; raw event persisted.", status: "passed" },
  { id: "job_01J8Z4", at: 58, source: "Worker", label: "Order reconciled", detail: "Current Stripe object hydrated and compared.", status: "passed" },
  { id: "db_01J8Z5", at: 78, source: "Database", label: "State converged", detail: "Merchant and Stripe state agree.", status: "passed" },
];

function scenarioEvents(kind: "duplicate" | "reorder" | "timeout" | "tamper"): RunEvent[] {
  const inserted: Record<typeof kind, RunEvent[]> = {
    duplicate: [
      { id: "evt_1Q9R2_dup", at: 48, source: "Webhook", label: "Duplicate detected", detail: "Same Stripe event ID arrived 614ms later.", status: "fault" },
      { id: "dedupe_842", at: 64, source: "Worker", label: "Side effect suppressed", detail: "Unique event constraint prevented double fulfillment.", status: "passed" },
    ],
    reorder: [
      { id: "evt_1Q9R1", at: 42, source: "Webhook", label: "Events out of order", detail: "invoice.paid arrived before invoice.created.", status: "fault" },
      { id: "hydrate_842", at: 66, source: "Worker", label: "Current state hydrated", detail: "Latest subscription object fetched before evaluation.", status: "passed" },
    ],
    timeout: [
      { id: "del_01J8Z6", at: 44, source: "Webhook", label: "Endpoint timed out", detail: "Target exceeded the 3s delivery budget.", status: "fault" },
      { id: "retry_02", at: 68, source: "Webhook", label: "Retry accepted", detail: "Second attempt acknowledged in 86ms.", status: "passed" },
    ],
    tamper: [
      { id: "sig_bad_01", at: 42, source: "Webhook", label: "Signature rejected", detail: "Payload digest did not match the signed body.", status: "fault" },
      { id: "quarantine_01", at: 61, source: "Worker", label: "Event quarantined", detail: "No business state mutation was allowed.", status: "passed" },
    ],
  };
  return [...baseEvents.slice(0, 4), ...inserted[kind], ...baseEvents.slice(4)].sort((a, b) => a.at - b.at);
}

export const scenarios: Scenario[] = [
  {
    slug: "duplicate-webhook",
    name: "Duplicate webhook",
    summary: "Deliver the same payment event twice and prove fulfillment occurs once.",
    faultAt: 48,
    duration: 92,
    evidence: "Unique constraint: stripe_event_id",
    expected: "1 fulfillment · $42.00 captured",
    observed: "1 fulfillment · $42.00 captured",
    events: scenarioEvents("duplicate"),
  },
  {
    slug: "event-disorder",
    name: "Event disorder",
    summary: "Reverse invoice delivery and verify state converges from the current object.",
    faultAt: 42,
    duration: 94,
    evidence: "Hydration: sub_1Q9 current state",
    expected: "Subscription active · invoice paid",
    observed: "Subscription active · invoice paid",
    events: scenarioEvents("reorder"),
  },
  {
    slug: "endpoint-timeout",
    name: "Endpoint timeout",
    summary: "Hold the endpoint open, recover, and accept Stripe's retry without loss.",
    faultAt: 44,
    duration: 96,
    evidence: "Delivery attempt 2 · 86ms",
    expected: "Event processed once after recovery",
    observed: "Event processed once after recovery",
    events: scenarioEvents("timeout"),
  },
  {
    slug: "tampered-payload",
    name: "Tampered payload",
    summary: "Mutate the raw request body and verify the event cannot change state.",
    faultAt: 42,
    duration: 88,
    evidence: "Signature mismatch · quarantined",
    expected: "Rejected · no state mutation",
    observed: "Rejected · no state mutation",
    events: scenarioEvents("tamper"),
  },
];

export function getVisibleEvents(scenario: Scenario, progress: number) {
  const elapsed = scenario.duration * Math.max(0, Math.min(1, progress));
  return scenario.events.filter((event) => event.at <= elapsed);
}

export function getRunState(scenario: Scenario, progress: number) {
  const elapsed = scenario.duration * Math.max(0, Math.min(1, progress));
  if (elapsed < scenario.faultAt) return "running" as const;
  const lastFault = Math.max(...scenario.events.filter((event) => event.status === "fault").map((event) => event.at));
  if (elapsed < lastFault + 16) return "diverged" as const;
  if (progress < 0.96) return "recovering" as const;
  return "verified" as const;
}

export const overview = {
  readiness: 94,
  delta: 7,
  lastRun: "2 minutes ago",
  findings: [
    { title: "Retry path recovered", detail: "Endpoint acknowledged attempt 2 in 86ms", tone: "verified" as const, time: "2m" },
    { title: "Invoice event disorder", detail: "Current subscription state prevented regression", tone: "warning" as const, time: "18m" },
    { title: "Stale signature blocked", detail: "Replay window rejected an event 8m old", tone: "verified" as const, time: "1h" },
  ],
};
