import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";

for (const width of [360, 1280]) test(`independent stores, API authorization and dashboard at ${width}px`, async ({ browser }) => {
  const contexts = await Promise.all([browser.newContext({ viewport: { width, height: 900 } }), browser.newContext({ viewport: { width, height: 900 } })]);
  const names = ["پوشاک نارون", "کیف و کفش آبان"];
  const storeIDs: string[] = [];
  const csrf: string[] = [];
  try {
    for (let i = 0; i < contexts.length; i++) {
      const page = await contexts[i].newPage();
      await page.goto("/login");
      const origin = new URL(page.url()).origin;
      const credentials = { username: `store_${randomUUID().replaceAll("-", "").slice(0, 12)}`, password: "store-password-12345" };
      expect((await page.request.post("/api/auth/register", { data: credentials })).status()).toBe(201);
      expect((await page.request.post("/api/auth/login", { headers: { Origin: origin }, data: credentials })).status()).toBe(200);
      const before = await (await page.request.get("/api/me")).json();
      expect(before.stores).toEqual([]); csrf.push(before.csrfToken);
      await page.goto("/dashboard");
      await expect(page.getByRole("heading", { name: "ساخت فروشگاه", exact: true })).toBeVisible();
      await expect(page.getByLabel("نام فروشگاه")).toBeVisible();
      expect((await page.request.post("/api/stores", { headers: { Origin: origin }, data: { name: "forbidden" } })).status()).toBe(403);
      expect((await page.request.post("/api/stores", { headers: { Origin: origin, "X-CSRF-Token": csrf[i] }, data: { name: "   " } })).status()).toBe(422);
      await page.getByLabel("نام فروشگاه").fill(names[i]);
      await page.getByLabel("نام فروشگاه").press("Tab");
      await expect(page.getByRole("button", { name: "ساخت فروشگاه", exact: true })).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(page.getByRole("status")).toContainText("ساخته شد");
      await expect(page.getByRole("heading", { name: "سفارش‌ها", exact: true })).toBeFocused();
      const after = await (await page.request.get("/api/me")).json();
      expect(after.stores).toHaveLength(1);
      expect(after.stores[0].name).toBe(names[i]);
      expect(after.stores[0].id).not.toBe(after.user.id);
      storeIDs.push(after.stores[0].id);
      const headers = { Origin: origin, "X-CSRF-Token": csrf[i] };
      const duplicates = await Promise.all([1, 2].map(() => page.request.post("/api/stores", { headers, data: { name: "duplicate" } })));
      expect(duplicates.map(r => r.status())).toEqual([409, 409]);
      await page.reload();
      await expect(page.locator(".shop-name")).toHaveText(names[i]);
      await expect(page.getByRole("heading", { name: "هنوز سفارشی ثبت نشده است" })).toBeVisible();
      await page.screenshot({ path: `test-results/store-orders-${width}-${i}.png`, fullPage: true });
      for (const [section, title] of [["customers", "مشتریان"], ["products", "محصولات"], ["attention", "نیازمند رسیدگی"], ["connection", "تنظیمات اتصال"]]) {
        if (width === 360) await page.getByRole("button", { name: "منوی فروشگاه" }).click();
        await page.getByRole("navigation").getByRole("link", { name: title, exact: true }).click();
        await expect(page).toHaveURL(new RegExp(`/dashboard/${section}$`));
        await expect(page.getByRole("heading", { name: title, exact: true })).toBeVisible();
        await expect(page.locator(".empty-work")).toBeVisible();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      }
      await expect(page.locator(".empty-work")).toContainText("اتصال پیج هنوز در این نسخه فعال نیست");
      await page.screenshot({ path: `test-results/store-connection-${width}-${i}.png`, fullPage: true });
    }
    expect(storeIDs[0]).not.toBe(storeIDs[1]);
    for (let i = 0; i < contexts.length; i++) {
      const page = contexts[i].pages()[0];
      expect((await page.request.get(`/api/stores/${storeIDs[i]}`)).status()).toBe(200);
      const other = await page.request.get(`/api/stores/${storeIDs[1-i]}`);
      expect(other.status()).toBe(404); expect(await other.text()).not.toContain(names[1-i]);
      await page.goto(`/dashboard?storeId=${storeIDs[1-i]}`);
      await expect(page.locator(".shop-name")).toHaveText(names[i]);
      await expect(page.locator("body")).not.toContainText(names[1-i]);
      expect((await (await page.request.get(`/api/me?storeId=${storeIDs[1-i]}`)).json()).stores.map((s: {id: string}) => s.id)).toEqual([storeIDs[i]]);
      const origin = new URL(page.url()).origin;
      expect((await page.request.post("/api/stores", { headers: { Origin: origin, "X-CSRF-Token": csrf[i] }, data: { name: "hijack", storeId: storeIDs[1-i] } })).status()).toBe(400);
      for (const invalid of ["unknown", "constructor", "toString"]) {
        expect((await page.request.get(`/dashboard/${invalid}`)).status()).toBe(404);
      }
    }
    const anonymous = await browser.newContext();
    try {
      const origin = new URL(contexts[0].pages()[0].url()).origin;
      expect((await anonymous.request.get(`${origin}/api/stores/${storeIDs[0]}`)).status()).toBe(401);
      expect((await anonymous.request.post(`${origin}/api/stores`, { headers: { Origin: origin }, data: { name: "anonymous" } })).status()).toBe(401);
    } finally { await anonymous.close(); }
  } finally { await Promise.all(contexts.map(c => c.close())); }
});

test("store loading, failed create and recovery do not claim success", async ({ page }) => {
  const credentials = { username: `retry_${randomUUID().replaceAll("-", "").slice(0, 12)}`, password: "store-password-12345" };
  await page.goto("/login");
  await page.request.post("/api/auth/register", { data: credentials });
  await page.request.post("/api/auth/login", { headers: { Origin: new URL(page.url()).origin }, data: credentials });
  let release!: () => void;
  const wait = new Promise<void>(resolve => { release = resolve; });
  await page.route("**/api/me", async route => { await wait; await route.abort(); });
  await page.goto("/dashboard");
  await expect(page.getByRole("status")).toContainText("در حال دریافت");
  release();
  await expect(page.locator(".shop-error")).toContainText("اطلاعات فروشگاه دریافت نشد");
  await page.unroute("**/api/me");
  await page.getByRole("button", { name: "تلاش دوباره" }).click();
  await page.getByLabel("نام فروشگاه").fill("فروشگاه آزمایش");
  let finish!: () => void;
  const pending = new Promise<void>(resolve => { finish = resolve; }); let calls = 0;
  await page.route("**/api/stores", async route => { calls++; await pending; await route.abort(); });
  await page.getByRole("button", { name: "ساخت فروشگاه", exact: true }).dblclick({ force: true });
  await expect(page.getByRole("button", { name: "در حال ساخت فروشگاه…" })).toBeDisabled();
  expect(calls).toBe(1); finish();
  await expect(page.locator(".shop-error")).toContainText("نتیجهٔ ساخت مشخص نیست");
  await expect(page.getByLabel("نام فروشگاه")).toHaveValue("فروشگاه آزمایش");
  await page.screenshot({ path: "test-results/store-error.png", fullPage: true });
  await page.unroute("**/api/stores");
  await page.getByRole("button", { name: "ساخت فروشگاه", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("ساخته شد");
});
