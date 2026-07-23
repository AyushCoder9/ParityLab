import { expect, test } from "@playwright/test";

const password = "ParityLab-browser-QA-42!";

test.describe.serial("authenticated product browser flow", () => {
  test.beforeEach(async ({ context }) => { await context.clearCookies(); });
  test("a protected route redirects to login and preserves the destination", async ({ page }) => {
    await page.goto("/settings");
    await expect(page).toHaveURL(/\/login\?next=%2Fsettings$/);
    await expect(page.getByRole("heading", { level: 1, name: /sign in to the control plane/i })).toBeVisible();
  });

  test("invalid login stays generic and does not create browser credentials", async ({ page }) => {
    await page.goto("/login");
    await page.getByLabel("Email address").fill(`missing-${crypto.randomUUID()}@example.test`);
    await page.getByLabel("Password").fill("definitely-incorrect-password");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    const alert = page.locator(".inline-error[role='alert']");
    await expect(alert).toContainText(/email or password is incorrect/i);
    await expect(alert).not.toContainText(/account|user.*exists/i);
    const browserState = await page.evaluate(() => ({ local: Object.keys(localStorage), session: Object.keys(sessionStorage), cookie: document.cookie }));
    expect(browserState.local.filter((key) => /auth|session|token/i.test(key))).toEqual([]);
    expect(browserState.session.filter((key) => /auth|session|token/i.test(key))).toEqual([]);
    expect(browserState.cookie).toBe("");
  });

  test("registration creates a protected session and logout revokes it", async ({ page }) => {
    const address = `browser-${crypto.randomUUID()}@example.test`;
    await page.goto("/login?next=%2Fsettings");
    await page.getByRole("tab", { name: "Create workspace" }).click();
    await page.getByLabel("Workspace name").fill("Browser QA Organization");
    await page.getByLabel("Project name").fill("Browser QA Project");
    await page.getByLabel("Email address").fill(address);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Create workspace", exact: true }).click();

    await expect(page).toHaveURL(/\/settings$/);
    await expect(page.getByRole("heading", { level: 1, name: /settings/i })).toBeVisible();
    await expect(page.getByText(address).filter({ visible: true }).first()).toBeVisible();
    expect(await page.locator("body").innerText()).not.toContain(password);
    const browserState = await page.evaluate(() => ({ local: Object.keys(localStorage), session: Object.keys(sessionStorage), cookie: document.cookie }));
    expect(browserState.local.filter((key) => /auth|session|token/i.test(key))).toEqual([]);
    expect(browserState.session.filter((key) => /auth|session|token/i.test(key))).toEqual([]);
    expect(browserState.cookie).not.toContain("paritylab_session");

    await page.getByRole("button", { name: "Account menu" }).filter({ visible: true }).click();
    await page.getByRole("menuitem", { name: "Sign out" }).filter({ visible: true }).click();
    await expect(page).toHaveURL(/\/login$/);
    await page.goto("/settings");
    await expect(page).toHaveURL(/\/login\?next=%2Fsettings$/);
  });

  test("session-check outage shows an explicit gate without substituting local identity", async ({ page }) => {
    await page.route("**/v1/session", (route) => route.abort("failed"));
    await page.goto("/dashboard");
    await expect(page.getByRole("heading", { level: 1, name: /could not verify your session/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /retry session check/i })).toBeVisible();
    await expect(page.getByText(/local owner/i)).toHaveCount(0);
  });
});
