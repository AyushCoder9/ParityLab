import {
  API_PATHS,
  type Fault,
  type ListResponse,
  type Overview,
  type Report,
  type Run,
  type RunEvent,
  type Scenario,
  type StripeConnection,
} from "@paritylab/contracts";

const API_ORIGIN = process.env.NEXT_PUBLIC_PARITYLAB_API_URL ?? "http://127.0.0.1:8080";

function apiURL(path: string) {
  return new URL(path, API_ORIGIN).toString();
}

export type StripeConnectionSummary = StripeConnection;

export async function checkEngine(signal?: AbortSignal) {
  const response = await fetch(apiURL(API_PATHS.health), { signal, cache: "no-store" });
  if (!response.ok) throw new Error(`Engine health returned ${response.status}`);
  const body = await response.json() as { status?: string; mode?: string };
  return body.status === "ok" && body.mode === "sandbox";
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(apiURL(path), { signal, cache: "no-store" });
  if (!response.ok) throw new Error(`ParityLab API returned ${response.status}`);
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
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ secret_key: input.secretKey, sandbox_name: input.sandboxName }),
  });
  if (!response.ok) throw new Error(await apiError(response));
  return response.json() as Promise<StripeConnectionSummary>;
}

export async function createStripePaymentIntentRun(input: { connectionID: string; amountMinor: number; currency: string; idempotencyKey: string; signal?: AbortSignal }): Promise<Run> {
  const response = await fetch(apiURL(API_PATHS.stripePaymentIntents), {
    method: "POST",
    signal: input.signal,
    cache: "no-store",
    headers: { "Content-Type": "application/json", "Idempotency-Key": input.idempotencyKey },
    body: JSON.stringify({ connection_id: input.connectionID, amount_minor: input.amountMinor, currency: input.currency }),
  });
  if (!response.ok) throw new Error(await apiError(response));
  return response.json() as Promise<Run>;
}

export const getEngineOverview = (signal?: AbortSignal) => getJSON<Overview>(API_PATHS.overview, signal);
export const getEngineScenarios = (signal?: AbortSignal) => getJSON<ListResponse<Scenario>>(API_PATHS.scenarios, signal);
export const getEngineRuns = (signal?: AbortSignal) => getJSON<ListResponse<Run>>(API_PATHS.runs, signal);
export const getEngineRun = (id: string, signal?: AbortSignal) => getJSON<Run>(API_PATHS.run(id), signal);
export const getEngineRunEvents = (id: string, signal?: AbortSignal) => getJSON<ListResponse<RunEvent>>(API_PATHS.events(id), signal);
export const getEngineReport = (id: string, signal?: AbortSignal) => getJSON<Report>(API_PATHS.report(id), signal);

export async function createEngineRun(input: {
  scenarioID: string;
  fault: Fault;
  idempotencyKey: string;
  signal?: AbortSignal;
}): Promise<Run> {
  const response = await fetch(apiURL(API_PATHS.runs), {
    method: "POST",
    signal: input.signal,
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": input.idempotencyKey,
    },
    body: JSON.stringify({ scenario_id: input.scenarioID, fault: input.fault }),
  });
  if (!response.ok) throw new Error(`Run creation returned ${response.status}`);
  return response.json() as Promise<Run>;
}
