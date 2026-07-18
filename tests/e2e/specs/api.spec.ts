import { expect, test } from "@playwright/test";
const apiURL = process.env.PARITYLAB_API_URL ?? "http://127.0.0.1:8080";

test.describe("public API contract", () => {
  test("health and scenarios are JSON and sandbox scoped", async ({ request }) => {
    const health = await request.get(`${apiURL}/healthz`);
    expect(health.ok()).toBeTruthy();
    expect(health.headers()["content-type"]).toContain("application/json");
    expect(await health.json()).toMatchObject({ status: "ok" });

    const response = await request.get(`${apiURL}/v1/scenarios`);
    expect(response.ok()).toBeTruthy();
    const scenarios = await response.json();
    expect(scenarios).toMatchObject({ object: "list", has_more: false, data: expect.any(Array) });
    expect(scenarios.data.length).toBeGreaterThan(2);
    expect(scenarios.data[0]).toEqual(expect.objectContaining({
      id: expect.any(String),
      name: expect.any(String),
      supported_faults: expect.any(Array),
      assertions: expect.any(Array)
    }));
  });

  test("create run is idempotent and reports conflicting reuse", async ({ request }) => {
    const key = `e2e-${test.info().workerIndex}-${Date.now()}`;
    const duplicateBody = { scenario_id: "checkout-duplicate", fault: "duplicate" };
    const first = await request.post(`${apiURL}/v1/runs`, {
      headers: { "Idempotency-Key": key },
      data: duplicateBody
    });
    expect(first.status()).toBe(201);
    const run = await first.json();
    expect(run).toEqual(expect.objectContaining({ id: expect.any(String), environment: "sandbox", status: "passed" }));

    const replay = await request.post(`${apiURL}/v1/runs`, {
      headers: { "Idempotency-Key": key },
      data: duplicateBody
    });
    expect(replay.status()).toBe(201);
    expect(replay.headers()["idempotent-replayed"]).toBe("true");
    expect((await replay.json()).id).toBe(run.id);

    const conflict = await request.post(`${apiURL}/v1/runs`, {
      headers: { "Idempotency-Key": key },
      data: { scenario_id: "checkout-duplicate", fault: "none" }
    });
    expect(conflict.status()).toBe(409);
    expect(await conflict.json()).toMatchObject({
      error: {
        type: expect.any(String),
        code: "idempotency_key_in_use",
        message: expect.any(String),
        request_id: expect.any(String)
      }
    });

    const report = await request.get(`${apiURL}/v1/runs/${run.id}/report`);
    expect(report.ok()).toBeTruthy();
    expect(await report.json()).toMatchObject({ run: { id: run.id }, assertions: expect.any(Array) });
  });

  test("mutations require an idempotency key", async ({ request }) => {
    const response = await request.post(`${apiURL}/v1/runs`, {
      data: { scenario_id: "checkout-duplicate", fault: "duplicate" }
    });
    expect(response.status()).toBe(400);
    expect(await response.json()).toMatchObject({ error: { code: "idempotency_key_missing" } });
  });
});
