import { expect, test, type Page, type Request } from "@playwright/test";

function collectMutations(page: Page) {
  const mutations: Request[] = [];
  page.on("request", (request) => {
    if (["POST", "PUT", "PATCH", "DELETE"].includes(request.method())) mutations.push(request);
  });
  return mutations;
}

test.describe("persisted product state boundaries", () => {
  test("settings save uses the protected API and survives a browser reload", async ({ page }) => {
    const mutations = collectMutations(page);
    await page.goto("/settings");
    await page.getByLabel("Project name").fill("Reliability review");
    await page.getByLabel("Evidence retention").selectOption("90");
    await page.getByRole("button", { name: "Save changes" }).click();

    await expect(page.getByRole("status")).toContainText(/saved to ParityLab/i);
    const writes = mutations.filter((request) => request.method() === "PATCH" && request.url().endsWith("/v1/settings/project"));
    expect(writes).toHaveLength(1);
    expect(writes[0].postDataJSON()).toEqual({ name: "Reliability review", retention_days: 90 });

    await page.reload();
    await expect(page.getByLabel("Project name")).toHaveValue("Reliability review");
    await expect(page.getByLabel("Evidence retention")).toHaveValue("90");
  });

  test("finding and notification controls send explicit persisted mutations", async ({ page }) => {
    const mutations = collectMutations(page);
    let resolved = false;
    let readAt: string | undefined;
    const finding = {
      id: "finding_browser_contract",
      run_id: "run_browser_contract",
      severity: "warning",
      title: "Duplicate delivery observed",
      summary: "The duplicate path was exercised.",
      cause: "A retry delivered the same event twice.",
      remediation: "Keep the merchant write idempotent.",
      checkpoint: "merchant.write",
      resolved,
    };
    await page.route(/\/v1\/findings(?:\/.*)?(?:\?.*)?$/, async (route) => {
      if (route.request().method() === "GET") {
        const url = new URL(route.request().url());
        const status = url.searchParams.get("status");
        const visible = !status || (status === "resolved" ? resolved : !resolved);
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ object: "list", data: visible ? [{ ...finding, resolved }] : [], has_more: false }) });
      }
      resolved = route.request().url().endsWith("/resolve");
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ...finding, resolved }) });
    });
    await page.route(/\/v1\/notifications(?:\/.*)?$/, async (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ object: "list", data: [{ id: "notification_browser_contract", kind: "run.completed", payload: { title: "Run completed", detail: "Evidence is ready." }, read_at: readAt, created_at: "2026-07-19T00:00:00Z" }], has_more: false }) });
      }
      readAt = "2026-07-19T00:01:00Z";
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ updated: 1 }) });
    });

    await page.goto("/findings");
    await page.getByRole("button", { name: "Mark resolved" }).click();
    await page.getByRole("button", { name: "Resolved", exact: true }).click();
    await expect(page.getByRole("button", { name: "Reopen finding" })).toBeVisible();

    await page.goto("/notifications");
    await page.getByRole("button", { name: "Mark all read (1)" }).click();
    await expect(page.getByText("All read", { exact: true })).toBeVisible();

    expect(mutations.some((request) => request.method() === "POST" && request.url().endsWith("/v1/findings/finding_browser_contract/resolve"))).toBe(true);
    expect(mutations.some((request) => request.method() === "POST" && request.url().endsWith("/v1/notifications/read-all"))).toBe(true);
  });

  test("environment selection is persisted through the protected API", async ({ page }) => {
    const mutations = collectMutations(page);
    await page.goto("/environments");
    await page.getByRole("button", { name: /Staging/ }).click();
    await expect(page.getByRole("status")).toContainText(/Staging selected/i);
    expect(mutations.some((request) => request.method() === "POST" && /\/v1\/environments\/[^/]+\/select$/.test(request.url()))).toBe(true);

    await page.reload();
    await expect(page.getByRole("button", { name: /Staging/ })).toHaveAttribute("aria-pressed", "true");
  });

  test("onboarding explains the secure API handoff without collecting a secret", async ({ page }) => {
    const mutations = collectMutations(page);
    await page.goto("/onboarding");
    await page.getByRole("button", { name: /continue/i }).click();

    await expect(page.getByRole("heading", { name: /enable encrypted connection storage/i })).toBeVisible();
    await expect(page.locator('input[type="password"]')).toHaveCount(0);
    await page.getByRole("button", { name: /continue to secure connection/i }).click();
    await expect(page.getByRole("heading", { name: /validate a restricted Sandbox key/i })).toBeVisible();
    await expect(page.getByText(/(posts|sends) the key directly to the API, clears the input immediately/i)).toBeVisible();
    await expect(page.locator('input[type="password"]')).toHaveCount(0);
    expect(mutations).toEqual([]);
  });
});

test.describe("mocked live run vertical slice", () => {
  test("Stripe connection returns only sanitized state and launches the frozen payment route", async ({ page }) => {
    const connectionID = "11111111-2222-4333-8444-555555555555";
    const testSecret = "rk_test_browser_contract_only";
    let connectionPayload: unknown;
    let paymentPayload: unknown;

    await page.route(/\/v1\/connections\/stripe\/validate$/, async (route) => {
      connectionPayload = route.request().postDataJSON();
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id: connectionID,
          stripe_account_id: "acct_mock_browser",
          sandbox_name: "Browser contract",
          status: "connected",
          created_at: "2026-07-19T00:00:00Z",
        }),
      });
    });
    await page.route(/\/v1\/stripe\/payment-intents$/, async (route) => {
      paymentPayload = route.request().postDataJSON();
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id: "run_900003",
          scenario_id: "checkout-duplicate",
          scenario_name: "Real Stripe PaymentIntent",
          fault: "duplicate",
          status: "passed",
          score: 96,
          started_at: "2026-07-19T00:00:00Z",
          completed_at: "2026-07-19T00:00:04Z",
          duration_ms: 4000,
          event_count: 6,
          finding_count: 1,
          recovered: true,
          environment: "sandbox",
          stripe_object_id: "pi_mock_browser",
          merchant_order_id: "ord_mock_browser",
        }),
      });
    });

    await page.goto("/connections");
    await page.getByLabel("Sandbox name").fill("Browser contract");
    await page.getByLabel("Restricted Sandbox secret key").fill(testSecret);
    await page.getByRole("button", { name: "Validate and connect" }).click();

    await expect(page.getByText("Connected account acct_mock_browser.")).toBeVisible();
    await expect(page.locator("body")).not.toContainText(testSecret);
    expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(testSecret);
    expect(connectionPayload).toEqual({ secret_key: testSecret, sandbox_name: "Browser contract" });

    await page.getByRole("button", { name: /Run real \$42 Sandbox payment/i }).click();
    await expect(page).toHaveURL(/\/runs\/run_900003$/);
    expect(paymentPayload).toEqual({ connection_id: connectionID, amount_minor: 4200, currency: "usd" });
  });

  test("successful API run creation displays the returned persisted ID", async ({ page }) => {
    let postedBody: unknown;
    let idempotencyKey = "";
    await page.route(/\/v1\/runs(?:\?.*)?$/, async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      postedBody = route.request().postDataJSON();
      idempotencyKey = await route.request().headerValue("Idempotency-Key") ?? "";
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id: "run_900001",
          scenario_id: "checkout-duplicate",
          scenario_name: "Duplicate checkout submission",
          fault: "duplicate",
          status: "passed",
          score: 96,
          started_at: "2026-07-19T00:00:00Z",
          completed_at: "2026-07-19T00:00:04Z",
          duration_ms: 4000,
          event_count: 6,
          finding_count: 1,
          recovered: true,
          environment: "sandbox",
          stripe_object_id: "pi_mock_900001",
          merchant_order_id: "ord_mock_900001",
        }),
      });
    });

    await page.goto("/demo");
    await expect(page.getByText(/RUN_900001/)).toBeVisible();
    expect(postedBody).toMatchObject({ scenario_id: "checkout-duplicate", fault: "duplicate" });
    expect(idempotencyKey).toMatch(/^paritylab-demo-/);
  });

  test("live ledger renders API records without relabeling them as seeded", async ({ page }) => {
    await page.route(/\/v1\/runs(?:\?.*)?$/, async (route) => {
      if (route.request().method() !== "GET") return route.continue();
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          object: "list",
          has_more: false,
          data: [{
            id: "run_900002",
            scenario_id: "checkout-duplicate",
            scenario_name: "Mock persisted checkout run",
            fault: "duplicate",
            status: "passed",
            score: 98,
            started_at: "2026-07-19T00:00:00Z",
            completed_at: "2026-07-19T00:00:03Z",
            duration_ms: 3000,
            event_count: 6,
            finding_count: 0,
            recovered: true,
            environment: "sandbox",
            stripe_object_id: "pi_mock_900002",
            merchant_order_id: "ord_mock_900002",
          }],
        }),
      });
    });

    await page.goto("/runs");
    await expect(page.getByText("Live run ledger", { exact: true })).toBeVisible();
    const row = page.getByRole("link", { name: /Mock persisted checkout run/ });
    await expect(row).toContainText("Live");
    await expect(row).toHaveAttribute("href", "/runs/run_900002");
    await expect(page.getByText(/Engine unavailable — showing seeded preview/i)).toHaveCount(0);
  });
});
