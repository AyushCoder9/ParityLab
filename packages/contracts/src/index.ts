export type Fault = "none" | "duplicate" | "reorder" | "timeout" | "tamper";
export type RunStatus = "running" | "passed" | "failed";
export type EventStatus = "healthy" | "diverged" | "recovered" | "blocked";

export interface Scenario {
  id: string;
  name: string;
  description: string;
  category: string;
  duration_ms: number;
  supported_faults: Fault[];
  assertions: string[];
  difficulty: string;
  recommended: boolean;
  estimated_event_count: number;
}

export interface Run {
  id: string;
  scenario_id: string;
  scenario_name: string;
  fault: Fault;
  status: RunStatus;
  score: number;
  started_at: string;
  completed_at: string;
  duration_ms: number;
  event_count: number;
  finding_count: number;
  recovered: boolean;
  environment: "sandbox";
  stripe_object_id: string;
  merchant_order_id: string;
}

export interface RunEvent {
  id: string;
  run_id: string;
  sequence: number;
  at: string;
  source: string;
  target: string;
  type: string;
  title: string;
  detail: string;
  status: EventStatus;
  latency_ms: number;
  checkpoint: string;
  trace_id: string;
  evidence?: Record<string, unknown>;
  is_duplicate?: boolean;
}

export interface Assertion {
  id: string;
  name: string;
  passed: boolean;
  expected: string;
  observed: string;
  evidence: string;
}

export interface Finding {
  id: string;
  severity: "info" | "warning" | "critical";
  title: string;
  summary: string;
  cause: string;
  remediation: string;
  checkpoint: string;
  resolved: boolean;
}

export interface Report {
  run: Run;
  summary: string;
  verdict: string;
  assertions: Assertion[];
  findings: Finding[];
  state: { stripe: string; webhook: string; merchant: string; balanced: boolean };
  generated_at: string;
}

export interface Overview {
  readiness_score: number;
  grade: string;
  environment: "sandbox";
  last_verified_at: string;
  stats: {
    total_runs: number;
    passed_runs: number;
    events_processed: number;
    duplicates_caught: number;
    p95_latency_ms: number;
  };
  categories: Array<{ id: string; label: string; score: number }>;
  recent_runs: Run[];
  critical_finding: Finding | null;
}

export interface ListResponse<T> {
  object: "list";
  data: T[];
  has_more: false;
}

export interface APIError {
  error: {
    type: string;
    code: string;
    message: string;
    param?: string;
    request_id: string;
  };
}

export const API_PATHS = {
  health: "/healthz",
  overview: "/v1/overview",
  scenarios: "/v1/scenarios",
  runs: "/v1/runs",
  run: (id: string) => `/v1/runs/${encodeURIComponent(id)}`,
  events: (id: string) => `/v1/runs/${encodeURIComponent(id)}/events`,
  report: (id: string) => `/v1/runs/${encodeURIComponent(id)}/report`,
} as const;
