import { expect, test } from "@playwright/test";

const apiURL = process.env.PARITYLAB_API_URL ?? "http://127.0.0.1:8080";
const enabled = process.env.PARITYLAB_STRIPE_VERTICAL_E2E === "1";

test("browser creates persisted Stripe-mock evidence through the live API", async ({ page, request }) => {
  test.skip(!enabled, "set PARITYLAB_STRIPE_VERTICAL_E2E=1 against the isolated Stripe-mock stack");

  await page.goto("/connections");
  await page.getByLabel("Sandbox name").fill("Browser to Postgres contract");
  await page.getByLabel("Restricted Sandbox secret key").fill("sk_test_browser_to_postgres_contract");
  await page.getByRole("button", { name: "Validate and connect" }).click();

  await expect(page.getByText("Connected account acct_mock_sandbox.")).toBeVisible();
  await page.getByRole("button", { name: /Run real \$42 Sandbox payment/i }).click();
  await expect(page).toHaveURL(/\/runs\/run_\d{6,}$/);
  await expect(page.getByText("Live run evidence", { exact: true })).toBeVisible();

  const runID = new URL(page.url()).pathname.split("/").at(-1);
  expect(runID).toMatch(/^run_\d{6,}$/);

  const runResponse = await request.get(`${apiURL}/v1/runs/${runID}`);
  expect(runResponse.ok()).toBeTruthy();
  const run = await runResponse.json();
  expect(run).toMatchObject({
    id: runID,
    environment: "sandbox",
    stripe_object_id: expect.stringMatching(/^pi_mock_/),
  });

  const reportResponse = await request.get(`${apiURL}/v1/runs/${runID}/report`);
  expect(reportResponse.ok()).toBeTruthy();
  const report = await reportResponse.json();
  expect(report).toMatchObject({
    run: { id: runID, stripe_object_id: run.stripe_object_id },
    state: { balanced: true },
    assertions: expect.arrayContaining([
      expect.objectContaining({
        id: "assert_minor_units",
        passed: true,
        expected: "4200 usd",
        observed: "4200 usd",
        evidence: run.stripe_object_id,
      }),
    ]),
  });
});
