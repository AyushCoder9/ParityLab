import { expect, test } from "@playwright/test";
import { MarketingPage } from "../pages/paritylab";

test.describe("marketing narrative", () => {
  test("communicates the product and offers direct product entry", async ({ page }) => {
    const marketing = new MarketingPage(page);
    await marketing.open();

    await expect(page).toHaveTitle(/ParityLab/i);
    await expect(page.getByRole("link", { name: /Run the simulation/i })).toHaveAttribute("href", "/demo");
    await expect(page.getByRole("link", { name: /Explore the console/i })).toHaveAttribute("href", "/dashboard");
    if ((page.viewportSize()?.width ?? 1280) <= 760) {
      await expect(page.getByRole("link", { name: "Open console" })).toBeVisible();
    } else {
      await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();
    }
    await expect(page.getByText("SANDBOX ONLY", { exact: false })).toBeVisible();
  });

  test("duplicate injection preserves the single business effect", async ({ page }) => {
    const marketing = new MarketingPage(page);
    await marketing.open();
    await marketing.injectDuplicate();

    const evidence = page.locator(".fault-evidence");
    await expect(evidence.getByText("02", { exact: true })).toBeVisible();
    await expect(evidence.getByText("business_effects", { exact: true })).toBeVisible();
    await expect(evidence.getByText("01", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Reset fault" })).toBeVisible();
  });

  test("architecture tabs expose their guarantee with keyboard controls", async ({ page }) => {
    const marketing = new MarketingPage(page);
    await marketing.open();

    const tablist = page.getByRole("tablist", { name: "Reliability architecture" });
    const outbox = tablist.getByRole("tab", { name: /Outbox/ });
    await outbox.focus();
    await outbox.press("Enter");
    await expect(outbox).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tabpanel")).toContainText("commit together");
  });
});
