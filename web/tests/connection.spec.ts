import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";

test("fake connection, callback replay, disconnect and reconnect preserve workspace", async ({ page }) => {
  await page.goto("/login");
  const origin = new URL(page.url()).origin;
  const credentials = { username: `conn_${randomUUID().slice(0, 12).replaceAll("-", "")}`, password: "connection-password-123" };
  expect((await page.request.post("/api/auth/register", { data: credentials })).status()).toBe(201);
  expect((await page.request.post("/api/auth/login", { headers: { Origin: origin }, data: credentials })).status()).toBe(200);
  const me = await (await page.request.get("/api/me")).json();
  const headers = { Origin: origin, "X-CSRF-Token": me.csrfToken };
  const { store } = await (await page.request.post("/api/stores", { headers, data: { name: "پوشاک نارون" } })).json();
  const base = `/api/stores/${store.id}/connection`;
  await page.route(`**${base}/status`, route => route.abort());
  await page.goto("/dashboard/connection");
  await expect(page.locator(".empty-work").getByRole("alert")).toContainText("وضعیت اتصال دریافت نشد");
  await page.unroute(`**${base}/status`);
  await page.getByRole("button", { name: "تلاش دوباره" }).click();
  await expect(page.getByRole("status")).toContainText("بدون اتصال");
  await page.getByRole("button", { name: "اتصال پیج آزمایشی", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status")).toContainText("متصل");
  const before = (await (await page.request.get(`${base}/status`)).json()).connection;
  const { state } = await (await page.request.post(`${base}/start`, { headers, data: {} })).json();
  const replay = { headers, data: { state, code: "demo-page" } };
  expect((await page.request.post(`${base}/callback`, replay)).status()).toBe(200);
  expect((await page.request.post(`${base}/callback`, replay)).status()).toBe(200);
  for (const width of [1280, 360]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(page.locator(".empty-work")).toContainText("محیط شبیه‌سازی‌شده");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    expect(await page.locator("html").getAttribute("dir")).toBe("rtl");
    await page.screenshot({ path: `test-results/connection-connected-${width}.png`, fullPage: true });
  }
  await page.getByRole("button", { name: "قطع اتصال آزمایشی" }).click();
  await expect(page.getByRole("status")).toContainText("قطع‌شده");
  expect((await page.request.get("/api/me")).status()).toBe(200);
  await page.screenshot({ path: "test-results/connection-disconnected-360.png", fullPage: true });
  await page.getByRole("link", { name: "بازگشت به سفارش‌ها" }).click();
  await expect(page.getByRole("heading", { name: "هنوز سفارشی ثبت نشده است" })).toBeVisible();
  await page.goto("/dashboard/connection");
  await page.getByRole("button", { name: "اتصال مجدد همین پیج" }).click();
  await expect(page.getByRole("status")).toContainText("متصل");
  await page.reload();
  await expect(page.getByRole("status")).toContainText("متصل");
  expect((await (await page.request.get(`${base}/status`)).json()).connection).toEqual(before);
  expect((await page.request.post(`${base}/disconnect`, { headers, data: {} })).status()).toBe(200);
});
