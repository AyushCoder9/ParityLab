import { expect, type Page } from "@playwright/test";

export class MarketingPage {
  constructor(private readonly page: Page) {}

  async open() {
    await this.page.goto("/");
    await expect(this.page.getByRole("heading", { level: 1 })).toContainText("survive");
  }

  async injectDuplicate() {
    await this.page.getByRole("button", { name: "Inject duplicate" }).click();
    await expect(this.page.getByText("held", { exact: true })).toBeVisible();
  }
}

export class DemoPage {
  constructor(private readonly page: Page) {}

  async open() {
    await this.page.goto("/demo");
    await expect(this.page.getByRole("main")).toBeVisible();
  }

  async selectExploreMode() {
    const explore = this.page.getByRole("button", { name: "Explore" });
    await explore.click();
    await expect(explore).toHaveAttribute("aria-pressed", "true");
  }
}

export class DashboardPage {
  constructor(private readonly page: Page) {}

  async open() {
    await this.page.goto("/dashboard");
    await expect(this.page.getByRole("main")).toBeVisible();
  }
}
