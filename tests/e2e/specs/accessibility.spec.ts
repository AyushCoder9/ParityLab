import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const routes = ["/", "/demo", "/dashboard"];

for (const route of routes) {
  test(`${route} has no serious accessibility violations`, async ({ page }) => {
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

test("principal navigation is keyboard reachable", async ({ page }) => {
  await page.goto("/");
  await page.keyboard.press("Tab");
  await expect(page.getByRole("link", { name: "ParityLab home" })).toBeFocused();
  await page.keyboard.press("Tab");
  const isMobile = (page.viewportSize()?.width ?? 1280) <= 760;
  await expect(page.getByRole("link", { name: isMobile ? "Open console" : "System" })).toBeFocused();
});
