import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const routes = [
  "/",
  "/demo",
  "/login",
  "/onboarding",
  "/dashboard",
  "/scenarios",
  "/runs",
  "/findings",
  "/reports",
  "/connections",
  "/environments",
  "/notifications",
  "/settings",
];

for (const route of routes) {
  test(`${route} has no serious accessibility violations`, async ({ context, page }) => {
    if (route === "/login") await context.clearCookies();
    await page.goto(route);
    await expect(page.getByRole("main")).toBeVisible();
    const results = await new AxeBuilder({ page }).analyze();
    const serious = results.violations.filter(({ impact }) => impact === "serious" || impact === "critical");
    expect(serious, JSON.stringify(serious, null, 2)).toEqual([]);
  });
}

test("reduced motion removes long-running CSS motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");
  const offenders = await page.locator("body *").evaluateAll((elements) =>
    elements.flatMap((element) => {
      const style = getComputedStyle(element);
      const animation = style.animationDuration.split(",").some((value) => Number.parseFloat(value) > 0.1);
      const transition = style.transitionDuration.split(",").some((value) => Number.parseFloat(value) > 0.4);
      return animation || transition ? [element.tagName + "." + element.className] : [];
    }).slice(0, 20)
  );
  expect(offenders, `Motion remains under prefers-reduced-motion: ${offenders.join(", ")}`).toEqual([]);
});

test("principal navigation is keyboard reachable", async ({ page, browserName }) => {
  // Real Safari/WebKit does not include <a> elements in the default Tab order
  // without "Full Keyboard Access" enabled — this isn't a Playwright quirk or
  // an app bug, it's actual platform behavior, so this check only applies to
  // engines where Tab-to-links reflects a real user's experience.
  test.skip(browserName === "webkit", "WebKit excludes links from the default Tab order without Full Keyboard Access");
  await page.goto("/");
  await page.keyboard.press("Tab");
  await expect(page.getByRole("link", { name: "ParityLab home" })).toBeFocused();
  await page.keyboard.press("Tab");
  const isMobile = (page.viewportSize()?.width ?? 1280) <= 760;
  await expect(page.getByRole("link", { name: isMobile ? "Open console" : "System" })).toBeFocused();
});

test("product routes fit a narrow viewport without horizontal overflow", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  for (const route of routes.filter((path) => !["/", "/demo"].includes(path))) {
    await page.goto(route);
    // /login (already-authenticated in this shared session) kicks off its own
    // getSession()-then-redirect-to-/dashboard effect; without waiting for it
    // to settle, that redirect can still be in flight when the next iteration
    // navigates elsewhere, aborting that navigation out from under us.
    await page.waitForLoadState("networkidle");
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `${route} overflows the mobile viewport`).toBeLessThanOrEqual(1);
  }
});
