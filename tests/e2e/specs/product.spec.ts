import { expect, test } from "@playwright/test";
import { DashboardPage, DemoPage } from "../pages/paritylab";

test("guided simulation exposes real controls and evidence", async ({ page }) => {
  const demo = new DemoPage(page);
  await demo.open();
  await demo.selectExploreMode();

  await expect(page.getByRole("heading", { level: 1, name: "Duplicate webhook" })).toBeVisible();
  await expect(page.getByRole("group", { name: "Demo mode" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Simulation playback" })).toBeVisible();
  await expect(page.getByRole("button", { name: /play simulation|pause simulation/i })).toBeVisible();
  await expect(page.getByRole("slider", { name: "Simulation progress" })).toBeVisible();
  await expect(page.getByRole("complementary", { name: "Scenarios" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Evidence" })).toBeVisible();
  await expect(page.getByText(/sandbox|simulated data/i).filter({ visible: true }).first()).toBeVisible();
  await expect(page.getByText(/RUN_000\d+ · duplicate-webhook/)).toBeVisible();
});

test("dashboard prioritizes readiness and running a simulation", async ({ page }) => {
  const dashboard = new DashboardPage(page);
  await dashboard.open();

  await expect(page.getByRole("heading", { level: 1, name: "Integration overview" })).toBeVisible();
  await expect(page.getByText(/readiness/i).first()).toBeVisible();
  await expect(page.getByRole("link", { name: /run simulation/i }).or(page.getByRole("button", { name: /run simulation/i }))).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Product navigation" })).toContainText(/Overview/i);
  await expect(page.getByRole("navigation", { name: "Product navigation" })).toContainText(/Findings/i);
  await expect(page.getByRole("button", { name: "Search or run a command" })).toBeVisible();
  await expect(page.getByText("Engine online")).toHaveCount(1);
});

test("dashboard fits a mobile viewport without horizontal overflow", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/dashboard");
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});
