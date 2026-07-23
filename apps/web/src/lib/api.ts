import {
  API_PATHS,
  type Fault,
  type Finding,
  type ListResponse,
  type Overview,
  type Report,
  type Run,
  type RunEvent,
  type Scenario,
  type StripeConnection,
} from "@paritylab/contracts";

const API_ORIGIN = process.env.NEXT_PUBLIC_PARITYLAB_API_URL ?? "http://127.0.0.1:8080";

export type SessionView = {
  authenticated: true;
  user: { id: string; email: string };
  organization: { id: string; name: string; role: string };
  project: { id: string; name: string; retention_days: number };
  expires_at?: string;
};

export class APIRequestError extends Error {
  constructor(message: string, public readonly status: number) { super(message); this.name = "APIRequestError"; }
}

function apiURL(path: string) {
  return new URL(path, API_ORIGIN).toString();
}

export function getRunEventStreamURL(id: string) {
  return apiURL(API_PATHS.events(id));
}

export type StripeConnectionSummary = StripeConnection;

export type ProjectSettings = {
  id: string;
  name: string;
  retention_days: number;
};

export type Environment = {
  id: string;
  name: string;
  kind: "local" | "sandbox" | "staging";
  is_default: boolean;
};

export type Notification = {
  id: string;
  run_id?: string;
  kind: string;
  payload: Record<string, unknown>;
  read_at?: string;
  created_at: string;
};

export async function checkEngine(signal?: AbortSignal) {
  const response = await fetch(apiURL(API_PATHS.health), { signal, cache: "no-store", credentials: "include" });
  if (!response.ok) throw new Error(`Engine health returned ${response.status}`);
  const body = await response.json() as { status?: string; mode?: string };
  return body.status === "ok" && body.mode === "sandbox";
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(apiURL(path), { signal, cache: "no-store", credentials: "include" });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
  return response.json() as Promise<T>;
}

async function mutateJSON<T>(path: string, input?: object, signal?: AbortSignal): Promise<T> {
  const response = await fetch(apiURL(path), {
    method: "POST",
    signal,
    cache: "no-store",
    credentials: "include",
    headers: input ? { "Content-Type": "application/json" } : undefined,
    body: input ? JSON.stringify(input) : undefined,
  });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
  return response.json() as Promise<T>;
}

async function patchJSON<T>(path: string, input: object, signal?: AbortSignal): Promise<T> {
  const response = await fetch(apiURL(path), {
    method: "PATCH",
    signal,
    cache: "no-store",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
  return response.json() as Promise<T>;
}

async function apiError(response: Response) {
  try {
    const body = await response.json() as { error?: { message?: string } };
    return body.error?.message ?? `ParityLab API returned ${response.status}`;
  } catch {
    return `ParityLab API returned ${response.status}`;
  }
}

export async function validateStripeConnection(input: { secretKey: string; sandboxName?: string; signal?: AbortSignal }): Promise<StripeConnectionSummary> {
  const response = await fetch(apiURL(API_PATHS.validateStripeConnection), {
    method: "POST",
    signal: input.signal,
    cache: "no-store",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ secret_key: input.secretKey, sandbox_name: input.sandboxName }),
  });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
  return response.json() as Promise<StripeConnectionSummary>;
}

export async function createStripePaymentIntentRun(input: { connectionID: string; amountMinor: number; currency: string; idempotencyKey: string; signal?: AbortSignal }): Promise<Run> {
  const response = await fetch(apiURL(API_PATHS.stripePaymentIntents), {
    method: "POST",
    signal: input.signal,
    cache: "no-store",
    credentials: "include",
    headers: { "Content-Type": "application/json", "Idempotency-Key": input.idempotencyKey },
    body: JSON.stringify({ connection_id: input.connectionID, amount_minor: input.amountMinor, currency: input.currency }),
  });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
  return response.json() as Promise<Run>;
}

export const getEngineOverview = (signal?: AbortSignal) => getJSON<Overview>(API_PATHS.overview, signal);
export const getEngineScenarios = (signal?: AbortSignal) => getJSON<ListResponse<Scenario>>(API_PATHS.scenarios, signal);
export const getEngineRuns = (signal?: AbortSignal) => getJSON<ListResponse<Run>>(API_PATHS.runs, signal);
export const getEngineRun = (id: string, signal?: AbortSignal) => getJSON<Run>(API_PATHS.run(id), signal);
export const getEngineRunEvents = (id: string, signal?: AbortSignal) => getJSON<ListResponse<RunEvent>>(API_PATHS.events(id), signal);
export const getEngineReport = (id: string, signal?: AbortSignal) => getJSON<Report>(API_PATHS.report(id), signal);
export const getProjectSettings = (signal?: AbortSignal) => getJSON<ProjectSettings>("/v1/settings/project", signal);
export const updateProjectSettings = (input: { name?: string; retention_days?: number }, signal?: AbortSignal) => patchJSON<ProjectSettings>("/v1/settings/project", input, signal);
export const getEnvironments = (signal?: AbortSignal) => getJSON<ListResponse<Environment>>("/v1/environments", signal);
export const selectEnvironment = (id: string, signal?: AbortSignal) => mutateJSON<Environment>(`/v1/environments/${encodeURIComponent(id)}/select`, undefined, signal);
export const getFindings = (status: "open" | "resolved" | "" = "", signal?: AbortSignal) => getJSON<ListResponse<Finding>>(`/v1/findings${status ? `?status=${status}` : ""}`, signal);
export const resolveFinding = (id: string, signal?: AbortSignal) => mutateJSON<Finding>(`/v1/findings/${encodeURIComponent(id)}/resolve`, undefined, signal);
export const reopenFinding = (id: string, signal?: AbortSignal) => mutateJSON<Finding>(`/v1/findings/${encodeURIComponent(id)}/reopen`, undefined, signal);
export const getNotifications = (signal?: AbortSignal) => getJSON<ListResponse<Notification>>("/v1/notifications", signal);
export const markNotificationRead = (id: string, signal?: AbortSignal) => mutateJSON<Notification>(`/v1/notifications/${encodeURIComponent(id)}/read`, undefined, signal);
export const markAllNotificationsRead = (signal?: AbortSignal) => mutateJSON<{ updated: number }>("/v1/notifications/read-all", undefined, signal);
export const getConnections = (signal?: AbortSignal) => getJSON<ListResponse<StripeConnectionSummary>>("/v1/connections", signal);

export async function createEngineRun(input: {
  scenarioID: string;
  fault: Fault;
  idempotencyKey: string;
  signal?: AbortSignal;
}): Promise<Run> {
  const response = await fetch(apiURL(API_PATHS.runs), {
    method: "POST",
    signal: input.signal,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": input.idempotencyKey,
    },
    body: JSON.stringify({ scenario_id: input.scenarioID, fault: input.fault }),
  });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
  return response.json() as Promise<Run>;
}

async function authRequest(path: string, input?: object): Promise<SessionView | null> {
  const response = await fetch(apiURL(path), {
    method: input ? "POST" : "GET",
    credentials: "include",
    cache: "no-store",
    headers: input ? { "Content-Type": "application/json" } : undefined,
    body: input ? JSON.stringify(input) : undefined,
    signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
  if (response.status === 204) return null;
  return response.json() as Promise<SessionView>;
}

export const getSession = () => authRequest("/v1/session") as Promise<SessionView>;
export const register = (input: { email: string; password: string; workspaceName: string; projectName: string }) => authRequest("/v1/auth/register", { email: input.email, password: input.password, workspace_name: input.workspaceName, project_name: input.projectName }) as Promise<SessionView>;
export const login = (input: { email: string; password: string }) => authRequest("/v1/auth/login", input) as Promise<SessionView>;
export async function logout() {
  const response = await fetch(apiURL("/v1/auth/logout"), { method: "POST", credentials: "include", cache: "no-store", signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new APIRequestError(await apiError(response), response.status);
}
