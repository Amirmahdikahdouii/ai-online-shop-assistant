import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
  timeout: 120_000,
  use: {
    baseURL: process.env.HEALTH_BASE_URL ?? "http://127.0.0.1:3100",
    trace: "retain-on-failure",
  },
});
