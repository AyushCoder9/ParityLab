import { expect, test, type Page } from "@playwright/test";

const productRoutes = [
  ["/login", /enter|sign in|log in/i],
  ["/onboarding", /onboarding|connect|workspace/i],
  ["/dashboard", /integration overview/i],
  ["/scenarios", /scenarios/i],
  ["/runs", /runs/i],
  ["/findings", /findings/i],
  ["/reports", /reports/i],
  ["/connections", /connections/i],
  ["/environments", /environments/i],
  ["/notifications", /notifications/i],
  ["/settings", /settings/i],
] as const;

async function expectRealScreen(page: Page, path: string, heading: RegExp) {
  const response = await page.goto(path);
  expect(response?.status(), `${path} should be an implemented route`).toBeLessThan(400);
  await expect(page.getByRole("main")).toBeVisible();
  await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
  await expect(page.getByText("Product demo", { exact: true })).toHaveCount(0);
  await expect(page.getByText(/represented in the seeded demo/i)).toHaveCount(0);
}

test.describe("real product route map", () => {
  for (const [path, heading] of productRoutes) {
    test(`${path} is a complete screen, not a placeholder`, async ({ context, page }) => {
      if (path === "/login") await context.clearCookies();
      await expectRealScreen(page, path, heading);
    });
  }

  test("product navigation consists of real routes", async ({ page }) => {
    await page.goto("/dashboard");
    const navigation = page.getByRole("navigation", { name: "Product navigation" });
    await expect(navigation).toBeVisible();

    const links = navigation.getByRole("link").filter({ visible: true });
    const isMobile = (page.viewportSize()?.width ?? 1280) <= 760;
    expect(await links.count(), "visible navigation items must be links, not local placeholder toggles").toBeGreaterThanOrEqual(isMobile ? 3 : 6);

    for (const link of await links.all()) {
      const href = await link.getAttribute("href");
      expect(href, "every product navigation item needs a destination").toMatch(/^\/(?!#)/);
    }
    if (isMobile) {
      await page.getByRole("button", { name: "More product navigation" }).click();
      const more = page.getByRole("dialog", { name: "More product navigation" });
      await expect(more).toBeVisible();
      await expect(more.getByRole("link", { name: /Findings/ })).toHaveAttribute("href", "/findings");
      await expect(more.getByRole("link", { name: /Connections/ })).toHaveAttribute("href", "/connections");
      await expect(more.getByRole("link", { name: /Settings/ }).first()).toHaveAttribute("href", "/settings");
    }
  });

  test("run and seeded report drill-downs are routable and labeled", async ({ page }) => {
    await page.goto("/runs");
    const runLink = page.locator('.data-table a[href^="/runs/"]').first();
    await expect(runLink).toBeVisible();
    const runHref = await runLink.getAttribute("href");
    await runLink.click();
    await expect(page).toHaveURL(/\/runs\//);
    if (runHref?.includes("seed_run_")) {
      await expect(page.getByRole("main")).toContainText(/seeded preview|seeded/i);
    } else {
      await expect(page.getByRole("main")).toContainText(/live run evidence|live evidence/i);
    }
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();

    await page.goto("/reports");
    const reportLink = page.locator('a[href^="/reports/seed_run_"]').first();
    await expect(reportLink).toBeVisible();
    await reportLink.click();
    await expect(page).toHaveURL(/\/reports\/seed_run_/);
    await expect(page.getByRole("main")).toContainText(/seeded preview|seeded/i);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  });
});

test.describe("truthful runtime and working controls", () => {
  test("API outage is explicit and never presented as live", async ({ page }) => {
    await page.route(/\/healthz(?:\?.*)?$/, (route) => route.abort("connectionfailed"));
    await page.route(/\/v1\//, (route) => route.abort("connectionfailed"));
    await page.goto("/dashboard");

    await expect(page.getByText(/Engine unavailable — showing seeded preview/i).first()).toBeVisible();
    await expect(page.getByText("Engine online", { exact: true })).toHaveCount(0);
    await expect(page.getByText(/live data/i)).toHaveCount(0);
  });

  test("notifications and account controls open working destinations", async ({ page }) => {
    await page.goto("/dashboard");

    const notifications = page.getByRole("link", { name: /notifications/i });
    await expect(notifications).toHaveAttribute("href", "/notifications");
    await notifications.click();
    await expect(page).toHaveURL(/\/notifications$/);
    await expect(page.getByRole("heading", { level: 1, name: /notifications/i })).toBeVisible();

    await page.goto("/dashboard");
    await page.getByRole("button", { name: /account menu/i }).filter({ visible: true }).click();
    await expect(page.getByRole("menu", { name: /account/i }).filter({ visible: true })).toBeVisible();
  });

  test("overview actions reach findings, runs, and evidence", async ({ page }) => {
    await page.goto("/dashboard");

    await expect(page.getByRole("link", { name: /view all/i })).toHaveAttribute("href", "/findings");
    await expect(page.getByRole("link", { name: /all runs/i })).toHaveAttribute("href", "/runs");

    const evidence = page.getByRole("link", { name: /inspect evidence/i });
    await expect(evidence).toHaveAttribute("href", /^\/(findings|runs|reports)(?:[/?]|$)/);
    await evidence.click();
    await expect(page).toHaveURL(/\/(findings|runs|reports)(?:[/?]|$)/);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("command palette executes navigation instead of only closing", async ({ page }) => {
    await page.goto("/dashboard");
    await page.getByRole("button", { name: "Search or run a command" }).click();
    const dialog = page.getByRole("dialog", { name: "Command palette" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: /configure stripe sandbox|connect stripe sandbox/i }).click();
    await expect(page).toHaveURL(/\/connections$/);
    await expect(page.getByRole("heading", { level: 1, name: /connections/i })).toBeVisible();
  });

  test("failed demo run creation does not invent a live run ID", async ({ page }) => {
    await page.route(/\/v1\/runs(?:\?.*)?$/, (route) => {
      if (route.request().method() === "POST") return route.abort("connectionfailed");
      return route.continue();
    });
    await page.goto("/demo");

    await expect(page.locator("main")).toContainText("SEED_PREVIEW");
    await expect(page.locator("main")).not.toContainText(/\bRUN_[A-Z0-9]+\b/);
  });
});
