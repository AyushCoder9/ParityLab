import type { Finding, Run } from "@paritylab/contracts";
import { scenarios } from "./simulation";

export const seededRuns: Run[] = scenarios.map((scenario, index) => ({
  id: `seed_run_01J8Z${index + 4}`,
  scenario_id: scenario.slug,
  scenario_name: scenario.name,
  fault: scenario.slug === "event-disorder" ? "reorder" : scenario.slug === "endpoint-timeout" ? "timeout" : scenario.slug === "tampered-payload" ? "tamper" : "duplicate",
  status: "passed",
  score: [98, 94, 91, 100][index] ?? 94,
  started_at: ["2026-07-19T08:58:00.000Z", "2026-07-19T08:42:00.000Z", "2026-07-19T08:00:00.000Z", "2026-07-19T06:00:00.000Z"][index],
  completed_at: ["2026-07-19T08:59:32.000Z", "2026-07-19T08:43:34.000Z", "2026-07-19T08:01:36.000Z", "2026-07-19T06:01:28.000Z"][index],
  duration_ms: scenario.duration * 1000,
  event_count: scenario.events.length,
  finding_count: index === 2 ? 1 : 0,
  recovered: index === 2,
  environment: "sandbox",
  stripe_object_id: `pi_seed_${index + 842}`,
  merchant_order_id: `order_seed_${index + 1842}`,
}));

export const seededFindings: Finding[] = [
  {
    id: "seed_finding_retry_01",
    severity: "warning",
    title: "Endpoint exceeded delivery budget",
    summary: "The first delivery timed out after 3.04 seconds; Stripe retry attempt 2 converged without a duplicate effect.",
    cause: "The reference target held the webhook connection past its configured three-second response budget.",
    remediation: "Acknowledge after signature and persistence, then process asynchronously.",
    checkpoint: "Webhook → Worker",
    resolved: false,
  },
  {
    id: "seed_finding_disorder_02",
    severity: "info",
    title: "Event disorder recovered",
    summary: "invoice.paid arrived before invoice.created; hydration prevented a state regression.",
    cause: "Webhook delivery order is not guaranteed.",
    remediation: "Hydrate the current Stripe object before applying state transitions.",
    checkpoint: "Stripe → Webhook",
    resolved: true,
  },
];

export const seededNotifications = [
  { id: "note_01", title: "Retry run converged", detail: "Endpoint timeout recovered on attempt 2.", time: "2m", unread: true },
  { id: "note_02", title: "Evidence report ready", detail: "Duplicate webhook report can be exported.", time: "18m", unread: true },
  { id: "note_03", title: "Sandbox check complete", detail: "Seeded reference target responded successfully.", time: "1h", unread: false },
];

export function formatRelativeDate(value: string) {
  const seconds = Math.max(1, Math.round((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
  return `${Math.round(seconds / 3600)}h ago`;
}
