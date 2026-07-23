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

export interface StripeConnection {
  id: string;
  stripe_account_id: string;
  sandbox_name: string;
  status: "connected";
  created_at: string;
}

export interface SessionView {
  authenticated: true;
  user: { id: string; email: string };
  organization: { id: string; name: string; role: "owner" | "admin" | "member" | "viewer" };
  project: ProjectSettings;
  expires_at: string;
}

export interface ProjectSettings {
  id: string;
  name: string;
  retention_days: number;
}

export interface Environment {
  id: string;
  name: string;
  kind: "local" | "sandbox" | "staging";
  is_default: boolean;
}

export interface Notification {
  id: string;
  run_id?: string;
  kind: string;
  payload: Record<string, unknown>;
  read_at?: string | null;
  created_at: string;
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
  validateStripeConnection: "/v1/connections/stripe/validate",
  stripePaymentIntents: "/v1/stripe/payment-intents",
  register: "/v1/auth/register",
  login: "/v1/auth/login",
  logout: "/v1/auth/logout",
  session: "/v1/session",
  projectSettings: "/v1/settings/project",
  environments: "/v1/environments",
  findings: "/v1/findings",
  notifications: "/v1/notifications",
  connections: "/v1/connections",
  run: (id: string) => `/v1/runs/${encodeURIComponent(id)}`,
  events: (id: string) => `/v1/runs/${encodeURIComponent(id)}/events`,
  report: (id: string) => `/v1/runs/${encodeURIComponent(id)}/report`,
  selectEnvironment: (id: string) => `/v1/environments/${encodeURIComponent(id)}/select`,
  resolveFinding: (id: string) => `/v1/findings/${encodeURIComponent(id)}/resolve`,
  reopenFinding: (id: string) => `/v1/findings/${encodeURIComponent(id)}/reopen`,
  markNotificationRead: (id: string) => `/v1/notifications/${encodeURIComponent(id)}/read`,
  markAllNotificationsRead: "/v1/notifications/read-all",
} as const;
