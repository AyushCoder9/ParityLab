import { API_PATHS, type Fault, type Run } from "@paritylab/contracts";

const API_ORIGIN = process.env.NEXT_PUBLIC_PARITYLAB_API_URL ?? "http://127.0.0.1:8080";

function apiURL(path: string) {
  return new URL(path, API_ORIGIN).toString();
}

export async function checkEngine(signal?: AbortSignal) {
  const response = await fetch(apiURL(API_PATHS.health), { signal, cache: "no-store" });
  if (!response.ok) throw new Error(`Engine health returned ${response.status}`);
  const body = await response.json() as { status?: string; mode?: string };
  return body.status === "ok" && body.mode === "sandbox";
}

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
