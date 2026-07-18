import { defineConfig, devices } from "@playwright/test";

const webURL = process.env.PARITYLAB_WEB_URL ?? "http://127.0.0.1:3000";
const apiURL = process.env.PARITYLAB_API_URL ?? "http://127.0.0.1:8080";
const externalServers = process.env.PARITYLAB_E2E_EXTERNAL_SERVERS === "1";

export default defineConfig({
  testDir: "./specs",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI
    ? [["line"], ["html", { open: "never", outputFolder: "artifacts/report" }], ["junit", { outputFile: "artifacts/junit.xml" }]]
    : [["list"], ["html", { open: "never", outputFolder: "artifacts/report" }]],
  outputDir: "artifacts/results",
  expect: { timeout: 7_500 },
  timeout: 30_000,
  use: {
    baseURL: webURL,
    actionTimeout: 10_000,
    navigationTimeout: 20_000,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    colorScheme: "light",
    locale: "en-US",
    timezoneId: "UTC"
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "webkit", use: { ...devices["Desktop Safari"] } },
    { name: "mobile-chrome", use: { ...devices["Pixel 7"] } }
  ],
  webServer: externalServers
    ? undefined
    : [
        {
          command: "go run ../../services/api/cmd/paritylab",
          cwd: __dirname,
          url: `${apiURL}/healthz`,
          reuseExistingServer: !process.env.CI,
          timeout: 120_000,
          env: { ...process.env, PORT: "8080" }
        },
        {
          command: "pnpm --dir ../.. dev",
          cwd: __dirname,
          url: webURL,
          reuseExistingServer: !process.env.CI,
          timeout: 120_000,
          env: { ...process.env, NEXT_PUBLIC_PARITYLAB_API_URL: apiURL }
        }
      ]
});

export { apiURL, webURL };
