import { expect, request as playwrightRequest, test, type APIRequestContext, type APIResponse } from "@playwright/test";
import { apiURL, webURL } from "../playwright.config";

const origin = process.env.PARITYLAB_WEB_URL ?? webURL;
const password = "ParityLab-QA-only-42!";

function email(label: string) {
  return `qa-${label}-${crypto.randomUUID()}@example.test`;
}

async function json(response: APIResponse) {
  const text = await response.text();
  return { text, value: text ? JSON.parse(text) as Record<string, unknown> : {} };
}

async function client(withOrigin = true) {
  return playwrightRequest.newContext({
    baseURL: apiURL,
    storageState: { cookies: [], origins: [] },
    extraHTTPHeaders: withOrigin ? { Origin: origin } : undefined
  });
}

async function register(api: APIRequestContext, address = email("member")) {
  const response = await api.post("/v1/auth/register", {
    data: { email: address, password, workspace_name: "QA Organization", project_name: "QA Project" }
  });
  const body = await json(response);
  expect(response.status(), body.text).toBe(201);
  expect(body.text).not.toContain(password);
  expect(body.value).not.toHaveProperty("password");
  expect(body.value).not.toHaveProperty("password_hash");
  return { address, body: body.value, response };
}

async function expectError(response: APIResponse, status: number, code?: string) {
  const body = await json(response);
  expect(response.status(), body.text).toBe(status);
  if (code) expect(body.value).toMatchObject({ error: { code } });
}

test.describe.serial("authentication and tenant security contract", () => {
  test("registration returns a sanitized session and hardened production cookie", async () => {
    const api = await client();
    const registered = await register(api, email("cookie"));
    const cookie = registered.response.headers()["set-cookie"] ?? "";

    expect(cookie).toMatch(/^paritylab_session=/);
    expect(cookie).toMatch(/(?:^|;\s*)Path=\//i);
    expect(cookie).toMatch(/(?:^|;\s*)HttpOnly(?:;|$)/i);
    expect(cookie).toMatch(/(?:^|;\s*)SameSite=Lax(?:;|$)/i);
    expect(cookie).toMatch(/(?:^|;\s*)Max-Age=86400(?:;|$)/i);
    if (process.env.PARITYLAB_EXPECT_SECURE_COOKIE !== "0") expect(cookie).toMatch(/(?:^|;\s*)Secure(?:;|$)/i);

    expect(registered.body).toMatchObject({
      user: { email: registered.address },
      organization: { name: "QA Organization", role: "owner" },
      project: { name: "QA Project" }
    });
    expect(registered.body).toHaveProperty("expires_at");
    await api.dispose();
  });

  test("protected resources reject missing and invalid sessions", async () => {
    const anonymous = await client();
    for (const path of [
      "/v1/session",
      "/v1/settings/project",
      "/v1/environments",
      "/v1/findings?status=all",
      "/v1/notifications",
      "/v1/connections"
    ]) await expectError(await anonymous.get(path), 401);
    await expectError(await anonymous.post("/v1/connections/stripe/validate", {
      data: { secret_key: "sk_test_must_not_reach_stripe", sandbox_name: "Anonymous" }
    }), 401);
    await expectError(await anonymous.post("/v1/stripe/payment-intents", {
      headers: { "Idempotency-Key": `anonymous-${crypto.randomUUID()}` },
      data: { connection_id: "conn_other_tenant", amount_minor: 4200, currency: "usd" }
    }), 401);
    await anonymous.dispose();

    const invalid = await playwrightRequest.newContext({
      baseURL: apiURL,
      storageState: { cookies: [], origins: [] },
      extraHTTPHeaders: { Origin: origin, Cookie: "paritylab_session=not-a-valid-session" }
    });
    await expectError(await invalid.get("/v1/session"), 401);
    await invalid.dispose();
  });

  test("cookie-authenticated mutations reject missing and foreign origins", async () => {
    const api = await client();
    await register(api, email("csrf"));

    const noOrigin = await api.patch("/v1/settings/project", {
      headers: { Origin: "" },
      data: { name: "must-not-save" }
    });
    await expectError(noOrigin, 403, "csrf_origin_invalid");

    const foreign = await api.patch("/v1/settings/project", {
      headers: { Origin: "https://attacker.invalid" },
      data: { name: "must-not-save" }
    });
    await expectError(foreign, 403, "csrf_origin_invalid");

    const current = await api.get("/v1/settings/project");
    expect(await current.json()).toMatchObject({ name: "QA Project" });
    await api.dispose();
  });

  test("logout revokes the prior session", async () => {
    const api = await client();
    const registered = await register(api, email("logout"));
    const oldCookie = (registered.response.headers()["set-cookie"] ?? "").split(";", 1)[0];
    const logout = await api.post("/v1/auth/logout");
    expect(logout.status()).toBe(204);
    expect(logout.headers()["set-cookie"] ?? "").toMatch(/paritylab_session=.*Max-Age=0/i);
    await expectError(await api.get("/v1/session"), 401);
    await api.dispose();

    const replay = await playwrightRequest.newContext({ baseURL: apiURL, storageState: { cookies: [], origins: [] }, extraHTTPHeaders: { Origin: origin, Cookie: oldCookie } });
    await expectError(await replay.get("/v1/session"), 401);
    await replay.dispose();
  });

  test("resources are tenant isolated even when another tenant knows an opaque ID", async () => {
    const tenantA = await client();
    const tenantB = await client();
    await register(tenantA, email("tenant-a"));
    await register(tenantB, email("tenant-b"));

    const environments = await tenantA.get("/v1/environments");
    expect(environments.status()).toBe(200);
    const list = await environments.json() as { data: Array<{ id: string }> };
    expect(list.data.length).toBeGreaterThanOrEqual(3);

    await expectError(await tenantB.post(`/v1/environments/${list.data[0].id}/select`), 404);
    const own = await tenantB.get("/v1/environments");
    const ownList = await own.json() as { data: Array<{ id: string }> };
    expect(ownList.data.map((item) => item.id)).not.toContain(list.data[0].id);
    await tenantA.dispose();
    await tenantB.dispose();
  });

  test("login is throttled without disclosing whether an account exists", async () => {
    const address = email("throttle");
    const missingAddress = email("missing");
    const api = await client();
    await register(api, address);
    await api.post("/v1/auth/logout");

    const knownFailure = await api.post("/v1/auth/login", { data: { email: address, password: "incorrect-password" } });
    const unknownFailure = await api.post("/v1/auth/login", { data: { email: missingAddress, password: "incorrect-password" } });
    const knownBody = await json(knownFailure);
    const unknownBody = await json(unknownFailure);
    expect(knownFailure.status(), knownBody.text).toBe(401);
    expect(unknownFailure.status(), unknownBody.text).toBe(401);
    expect(knownBody.value).toMatchObject({ error: { code: "invalid_credentials" } });
    const publicError = (value: Record<string, unknown>) => {
      const error = value.error as Record<string, unknown>;
      return { type: error.type, code: error.code, message: error.message, param: error.param };
    };
    expect(publicError(unknownBody.value)).toEqual(publicError(knownBody.value));

    let limited: APIResponse | undefined;
    for (let attempt = 0; attempt < 12; attempt += 1) {
      const response = await api.post("/v1/auth/login", { data: { email: address, password: "incorrect-password" } });
      if (response.status() === 429) { limited = response; break; }
      expect(response.status()).toBe(401);
      const body = await response.text();
      expect(body.toLowerCase()).not.toContain("exists");
    }
    expect(limited, "the login limiter must reject a deterministic burst").toBeDefined();
    expect(limited?.headers()["retry-after"]).toMatch(/^\d+$/);
    await expectError(limited!, 429, "rate_limit_exceeded");
    await api.dispose();
  });
});
