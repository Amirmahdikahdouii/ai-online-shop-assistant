import { expect, test, type APIRequestContext } from "@playwright/test";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { randomUUID } from "node:crypto";

const root = path.resolve(__dirname, "../..");
async function healthStatus(request: APIRequestContext) {
  try {
    return (await request.get("/api/health")).status();
  } catch {
    return 0;
  } // Existing keep-alive sockets can reset during container recreation.
}
function compose(...args: string[]) {
  if (!process.env.SMOKE_PROJECT?.startsWith("shop-smoke"))
    throw new Error("Run via scripts/smoke.sh");
  return execFileSync(
    "docker",
    [
      "compose",
      "--env-file",
      "config/demo.env",
      "-p",
      process.env.SMOKE_PROJECT,
      ...args,
    ],
    {
      cwd: root,
      env: { ...process.env, WEB_PORT: "3100" },
      encoding: "utf8",
      timeout: 90_000,
    },
  );
}

test("browser → API → PostgreSQL, migrations, restart persistence and outages", async ({
  page,
  request,
}) => {
  const configuration = JSON.parse(compose("config", "--format", "json"));
  expect(configuration.services.postgres.ports).toBeUndefined();
  expect(configuration.services.api.ports).toBeUndefined();
  expect(configuration.services.worker.ports).toBeUndefined();
  expect(
    configuration.services.web.environment.POSTGRES_PASSWORD,
  ).toBeUndefined();
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "برنامه آماده است", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("محیط نمایشی", { exact: true })).toBeVisible();
  expect(await page.locator("html").getAttribute("dir")).toBe("rtl");
  const before = await (await request.get("/api/health")).json();
  expect(before.database).toBe("ready");
  expect(before.worker).toBe("ready");
  expect(before.checks).toBeGreaterThan(0);
  const external: string[] = [];
  page.on("request", (req) => {
    if (new URL(req.url()).hostname !== "127.0.0.1") external.push(req.url());
  });
  await page.reload();
  await page.evaluate(() => document.fonts.ready);
  expect(
    await page.evaluate(() => document.fonts.check("16px Vazirmatn")),
  ).toBe(true);
  expect(external).toEqual([]);
  await page.screenshot({
    path: "test-results/health-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 375, height: 812 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.getByRole("button", { name: "بررسی دوباره" }).focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("heading", { name: "برنامه آماده است", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/health-mobile.png",
    fullPage: true,
  });

  // Recreate containers, preserving the named volume (not just process restart).
  const marker = randomUUID();
  compose(
    "exec",
    "-T",
    "postgres",
    "psql",
    "-U",
    "shop",
    "-d",
    "shop_demo",
    "-c",
    `INSERT INTO infrastructure.health_probe(id, checks) VALUES ('${marker}', 987654321)`,
  );
  compose("down");
  compose("up", "-d", "--wait");
  await expect.poll(() => healthStatus(request)).toBe(200);
  const after = await (await request.get("/api/health")).json();
  expect(after.probeId).toBe(before.probeId);
  expect(after.checks).toBeGreaterThan(before.checks);
  expect(
    compose(
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      "shop",
      "-d",
      "shop_demo",
      "-Atc",
      `SELECT checks FROM infrastructure.health_probe WHERE id='${marker}'`,
    ).trim(),
  ).toBe("987654321");

  try {
    compose("stop", "worker");
    await expect
      .poll(
        async () => (await (await request.get("/api/health")).json()).worker,
        { timeout: 20_000 },
      )
      .toBe("preparing");
    expect((await request.get("/api/health")).status()).toBe(503);
  } finally {
    compose("start", "worker");
  }
  await expect.poll(() => healthStatus(request)).toBe(200);

  try {
    compose(
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      "shop",
      "-d",
      "shop_demo",
      "-c",
      "ALTER TABLE infrastructure.health_probe RENAME COLUMN checks TO broken",
    );
    expect((await request.get("/api/health")).status()).toBe(503);
  } finally {
    compose(
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      "shop",
      "-d",
      "shop_demo",
      "-c",
      "ALTER TABLE infrastructure.health_probe RENAME COLUMN broken TO checks",
    );
  }
  await expect.poll(() => healthStatus(request)).toBe(200);

  try {
    compose("stop", "postgres");
    await page.getByRole("button", { name: "بررسی دوباره" }).click();
    await expect(
      page.getByRole("heading", {
        name: "برنامه هنوز آماده نیست",
        exact: true,
      }),
    ).toBeVisible();
    expect((await request.get("/api/health")).status()).toBe(503);
    await page.screenshot({
      path: "test-results/health-db-error.png",
      fullPage: true,
    });
  } finally {
    compose("start", "postgres");
  }
  await expect
    .poll(() => healthStatus(request), {
      timeout: 30_000,
    })
    .toBe(200);
  try {
    compose("stop", "api");
    const unavailable = await request.get("/api/health");
    expect(unavailable.status()).toBe(503);
    expect((await unavailable.json()).api).toBe("error");
    await page.getByRole("button", { name: "بررسی دوباره" }).click();
    await expect(
      page.getByRole("heading", { name: "برنامه هنوز آماده نیست" }),
    ).toBeVisible();
  } finally {
    compose("start", "api");
  }
  await expect.poll(() => healthStatus(request)).toBe(200);
  // A known server-only password must never appear in HTML, browser assets or logs.
  const sources = await page
    .locator("script[src]")
    .evaluateAll((nodes) => nodes.map((n) => (n as HTMLScriptElement).src));
  for (const src of sources)
    expect(await (await request.get(src)).text()).not.toContain(
      "local-demo-only",
    );
  expect(await page.content()).not.toContain("local-demo-only");
  expect(compose("logs", "--no-color")).not.toContain("local-demo-only");
});

test("loading, unavailable API and keyboard retry", async ({ page }) => {
  let release!: () => void;
  const waiting = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/health", async (route) => {
    await waiting;
    await route.fulfill({
      status: 503,
      json: {
        status: "error",
        api: "error",
        database: "unknown",
        worker: "unknown",
      },
    });
  });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "در حال بررسی ارتباط‌ها…" }),
  ).toBeVisible();
  await expect(page.getByRole("button")).toBeDisabled();
  await expect(page.getByText("هنوز نتیجه‌ای ثبت نشده است.")).toBeVisible();
  await page.screenshot({
    path: "test-results/health-loading.png",
    fullPage: true,
  });
  release();
  await expect(
    page.getByRole("heading", { name: "برنامه هنوز آماده نیست" }),
  ).toBeVisible();
  await page.getByRole("button").focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button")).toBeEnabled();
});
