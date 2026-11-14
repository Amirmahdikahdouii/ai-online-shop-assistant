import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";

test("durable sample statuses, dedupe, empty, loading, error and responsive ledger", async ({ page }) => {
  await page.goto("/login");
  const origin = new URL(page.url()).origin;
  const credentials = { username: `msg_${randomUUID().slice(0, 12).replaceAll("-", "")}`, password: "processing-password-123" };
  expect((await page.request.post("/api/auth/register", { data: credentials })).status()).toBe(201);
  expect((await page.request.post("/api/auth/login", { headers: { Origin: origin }, data: credentials })).status()).toBe(200);
  const me = await (await page.request.get("/api/me")).json();
  const headers = { Origin: origin, "X-CSRF-Token": me.csrfToken };
  const { store } = await (await page.request.post("/api/stores", { headers, data: { name: "پیام‌های فروشگاه نارون" } })).json();
  const base = `/api/stores/${store.id}`;
  await page.route(`**${base}/processing`, async route => { await new Promise(resolve => setTimeout(resolve, 900)); await route.continue(); });
  await page.goto("/dashboard/processing");
  await expect(page.getByText("در حال دریافت وضعیت پیام‌ها…")).toBeVisible();
  await expect(page.getByText("هنوز پیامی دریافت نشده است.", { exact: false })).toBeVisible();
  await page.unroute(`**${base}/processing`);
  await page.getByRole("button", { name: "دریافت پیام نمونه", exact: true }).click();
  await expect(page.locator(".message-ledger").getByRole("alert")).toContainText("ابتدا پیج آزمایشی");
  // A successful background refresh must not erase an unresolved submission error.
  await page.waitForResponse(r => r.url().endsWith(`${base}/processing`) && r.status() === 200);
  await expect(page.locator(".message-ledger").getByRole("alert")).toContainText("ابتدا پیج آزمایشی");
  const { state } = await (await page.request.post(`${base}/connection/start`, { headers, data: {} })).json();
  expect((await page.request.post(`${base}/connection/callback`, { headers, data: { state, code: "demo-page" } })).status()).toBe(200);
  await page.getByRole("button", { name: "دریافت پیام نمونه", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(page.locator(".message-list li")).toHaveCount(1);
  await page.getByRole("button", { name: "تکرار آخرین پیام" }).click();
  await expect(page.getByRole("status")).toContainText("کار دوم ساخته نشد");
  await expect(page.locator(".message-list li")).toHaveCount(1);
  await expect(page.locator(".message-list")).toContainText("پردازش موفق", { timeout: 15000 });
  await page.getByRole("button", { name: "نمونهٔ پیش از اتصال" }).click();
  await expect(page.getByRole("status")).toContainText("کنار گذاشته شد");
  await expect(page.locator(".message-list li")).toHaveCount(1);
  await page.getByRole("button", { name: "تکرار آخرین پیام" }).click();
  await expect(page.getByRole("status")).toContainText("کنار گذاشته شد");
  await expect(page.locator(".message-list li")).toHaveCount(1);
  const worker = `${process.env.SMOKE_PROJECT || "shop-smoke"}-worker-1`;
  execFileSync("docker", ["pause", worker]);
  try {
    await page.getByRole("button", { name: "نمونهٔ خطای پردازش" }).click();
    await expect(page.locator(".message-list")).toContainText("در انتظار پردازش");
    await page.screenshot({ path: "test-results/processing-pending.png", fullPage: true });
  } finally { execFileSync("docker", ["unpause", worker]); }
  await page.getByRole("button", { name: "تکرار آخرین پیام" }).click();
  await expect(page.getByRole("status")).toContainText("کار دوم ساخته نشد");
  await expect(page.locator(".message-list li")).toHaveCount(2);
  await expect(page.locator(".message-list")).toContainText("پردازش ناموفق", { timeout: 20000 });
  for (const width of [1280, 360]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: `test-results/processing-${width}.png`, fullPage: true });
  }
  await page.route(`**${base}/processing`, route => route.abort());
  await expect(page.locator(".message-ledger").getByRole("alert")).toContainText("وضعیت پیام‌ها دریافت نشد");
  await page.unroute(`**${base}/processing`);
  await page.getByRole("button", { name: "تلاش دوباره" }).click();
  await expect(page.locator(".message-ledger").getByRole("alert")).toHaveCount(0);
  expect((await page.request.get("/api/stores/not-my-store/processing")).status()).toBe(404);
  expect((await page.request.post(`${base}/sample-events`, { headers: { Origin: origin }, data: { scenario: "message", key: randomUUID() } })).status()).toBe(403);
  expect((await page.request.post(`${base}/sample-events`, { headers, data: { scenario: "message", key: randomUUID(), speaker: "shop" } })).status()).toBe(400);
  expect((await page.request.post(`${base}/sample-events`, { headers, data: { scenario: "message|failure", key: randomUUID() } })).status()).toBe(422);
  expect((await page.request.post(`${base}/connection/disconnect`, { headers, data: {} })).status()).toBe(200);
});
