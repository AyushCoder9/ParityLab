import { request, type FullConfig } from "@playwright/test";

export default async function globalSetup(config: FullConfig) {
  const apiURL = process.env.PARITYLAB_API_URL ?? "http://127.0.0.1:8080";
  const webURL = process.env.PARITYLAB_WEB_URL ?? "http://127.0.0.1:3000";
  const statePath = `${config.configDir}/artifacts/auth-state.json`;
  const api = await request.newContext({ baseURL: apiURL, extraHTTPHeaders: { Origin: webURL } });
  const response = await api.post("/v1/auth/register", {
    data: {
      email: `playwright-${crypto.randomUUID()}@example.test`,
      password: "ParityLab-Playwright-QA-42!",
      workspace_name: "Playwright workspace",
      project_name: "Playwright project"
    }
  });
  if (response.status() !== 201) throw new Error(`Playwright auth setup failed (${response.status()}): ${await response.text()}`);
  await api.storageState({ path: statePath });
  await api.dispose();
  process.env.PARITYLAB_E2E_AUTH_STATE = statePath;
}
