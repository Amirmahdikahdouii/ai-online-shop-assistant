import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import path from "node:path";

const password = "a-safe-registration-password-۱۴۰۵";
const suffix = () => randomUUID().replaceAll("-", "").slice(0, 12);
function sql(query: string) {
  if (!process.env.SMOKE_PROJECT?.startsWith("shop-smoke")) throw new Error("Run via scripts/smoke.sh");
  return execFileSync("docker", ["compose", "--env-file", "config/demo.env", "-p", process.env.SMOKE_PROJECT, "exec", "-T", "postgres", "psql", "-U", "shop", "-d", "shop_demo", "-Atc", query], {
    cwd: path.resolve(__dirname, "../.."), env: { ...process.env, WEB_PORT: "3100" }, encoding: "utf8",
  }).trim();
}

test("registration API persists only salted hashes, normalizes and arbitrates concurrent duplicates", async ({ request }) => {
  const name = `Shop_${suffix()}`;
  const responses = await Promise.all([name, `  ${name.toUpperCase()}  `].map(username => request.post("/api/auth/register", { data: { username, password } })));
  expect(responses.map(r => r.status()).sort()).toEqual([201, 409]);
  const created = await responses.find(r => r.status() === 201)!.json();
  expect(created.user.username).toBe(name.toLowerCase());
  expect(created.user.id).toBeTruthy();
  expect(Date.parse(created.user.createdAt)).not.toBeNaN();
  expect(JSON.stringify(created)).not.toContain(password);
  expect(created.user.password_hash).toBeUndefined();
  const hash = sql(`SELECT password_hash FROM identity.users WHERE username='${name.toLowerCase()}'`);
  expect(hash).toMatch(/^\$argon2id\$v=19\$m=65536,t=3,p=1\$/);
  expect(hash).not.toContain(password);
  const other = `shop_${suffix()}`;
  expect((await request.post("/api/auth/register", { data: { username: other, password } })).status()).toBe(201);
  expect(sql(`SELECT password_hash FROM identity.users WHERE username='${other}'`)).not.toBe(hash);
  const persian = `كيان_${suffix()}`;
  expect((await request.post("/api/auth/register", { data: { username: persian, password } })).status()).toBe(201);
  expect((await request.post("/api/auth/register", { data: { username: persian.replace("ك", "ک").replace("ي", "ی"), password } })).status()).toBe(409);
  for (const username of ["", "ab", "bad name", "bad\u202ename", "...", "x".repeat(33)]) {
    const response = await request.post("/api/auth/register", { data: { username, password: "short" } });
    expect(response.status()).toBe(422);
    expect((await response.json()).fieldErrors).toHaveProperty("username");
  }
  expect((await request.post("/api/auth/register", { data: { username: other, password, email: "not-accepted@example.com" } })).status()).toBe(400);
  expect((await request.post("/api/auth/register", { data: "{", headers: { "Content-Type": "application/json" } })).status()).toBe(400);
  expect((await request.post("/api/auth/register", { data: "x".repeat(5000), headers: { "Content-Type": "application/json" } })).status()).toBe(413);
  expect((await request.post("/api/auth/register", { data: { username: other, password }, headers: { Origin: "https://elsewhere.invalid" } })).status()).toBe(403);
  for (const route of ["recover", "reset-password", "sms", "verify-email"]) expect((await request.post(`/api/auth/${route}`, { data: {} })).status()).toBe(404);
});

for (const width of [360, 1280]) for (const script of ["fa", "en"]) test(`registration browser, RTL, keyboard and real success at ${width}px ${script}`, async ({ page }) => {
  await page.setViewportSize({ width, height: 900 });
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  const external: string[] = [];
  page.on("request", r => { if (new URL(r.url()).origin !== new URL(page.url()).origin && page.url() !== "about:blank") external.push(r.url()); });
  await page.goto("/register");
  await page.evaluate(() => document.fonts.ready);
  expect(await page.evaluate(() => document.fonts.check("16px Vazirmatn"))).toBe(true);
  expect(await page.locator("html").getAttribute("dir")).toBe("rtl");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: `test-results/register-${width}-${script}-empty.png`, fullPage: true });
  await page.getByRole("button", { name: "ساخت حساب", exact: true }).click();
  await expect(page.locator("#username")).toBeFocused();
  await expect(page.locator("#password-error")).toBeVisible();
  const username = `${script === "fa" ? "نارون" : "Narvan"}_${suffix()}`;
  await page.getByLabel("نام کاربری", { exact: true }).fill(username);
  await page.getByLabel("رمز عبور", { exact: true }).fill(password);
  await page.getByRole("button", { name: "ساخت حساب", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "حساب شما ساخته شد" })).toBeFocused();
  await expect(page.locator(".registered-name")).toHaveText(username.toLowerCase());
  expect(await page.locator('input[type="password"]').count()).toBe(0);
  await page.screenshot({ path: `test-results/register-${width}-${script}-success.png`, fullPage: true });
  await page.reload();
  await page.locator("#username").fill(username.toUpperCase());
  await page.locator("#password").fill(password);
  await page.getByRole("button", { name: "ساخت حساب", exact: true }).click();
  await expect(page.locator("#username-error")).toContainText("قبلاً ثبت شده");
  await expect(page.locator("#username")).toBeFocused();
  await page.screenshot({ path: `test-results/register-${width}-${script}-duplicate.png`, fullPage: true });
  expect(external).toEqual([]);
});

test("loading prevents double submission; network and service failures never report success", async ({ page }) => {
  let release!: () => void;
  const waiting = new Promise<void>(resolve => { release = resolve; });
  let requests = 0;
  await page.route("**/api/auth/register", async route => {
    requests++; await waiting; await route.abort("failed");
  });
  await page.goto("/register");
  await page.locator("#username").fill(`retry_${suffix()}`);
  await page.locator("#password").fill(password);
  await page.getByRole("button", { name: "ساخت حساب", exact: true }).dblclick({ force: true });
  await expect(page.locator('button[type="submit"]')).toBeDisabled();
  await expect(page.locator("form")).toHaveAttribute("aria-busy", "true");
  await page.screenshot({ path: "test-results/register-loading.png", fullPage: true });
  expect(requests).toBe(1);
  release();
  await expect(page.locator(".register-error")).toContainText("نتیجهٔ ثبت‌نام مشخص نیست");
  await expect(page.locator("#password")).toHaveValue(password);
  await page.screenshot({ path: "test-results/register-network-error.png", fullPage: true });
  await page.unroute("**/api/auth/register");
  await page.route("**/api/auth/register", route => route.fulfill({ status: 503, json: { code: "registration_unavailable" } }));
  await page.getByRole("button", { name: "ساخت حساب", exact: true }).click();
  await expect(page.locator(".register-error")).toContainText("ثبت‌نام تأیید نشد");
  await expect(page.getByRole("heading", { name: "حساب شما ساخته شد" })).toHaveCount(0);
  await page.unroute("**/api/auth/register");
  await page.getByRole("button", { name: "ساخت حساب", exact: true }).click();
  await expect(page.getByRole("heading", { name: "حساب شما ساخته شد" })).toBeVisible();
});
