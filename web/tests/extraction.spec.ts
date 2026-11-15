import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";

for (const width of [360, 1280]) test(`caption extraction, evidence and review at ${width}px`, async ({ page }) => {
 await page.setViewportSize({ width, height: 960 });
 await page.goto("/login");
 const origin = new URL(page.url()).origin;
 const credentials = { username: `ext_${randomUUID().replaceAll("-", "").slice(0, 12)}`, password: "extraction-password-123" };
 expect((await page.request.post("/api/auth/register", { data: credentials })).status()).toBe(201);
 expect((await page.request.post("/api/auth/login", { headers: { Origin: origin }, data: credentials })).status()).toBe(200);
 const me = await (await page.request.get("/api/me")).json();
 const headers = { Origin: origin, "X-CSRF-Token": me.csrfToken };
 const { store } = await (await page.request.post("/api/stores", { headers, data: { name: "پوشاک نارون" } })).json();
 const base = `/api/stores/${store.id}`;
 const { state } = await (await page.request.post(`${base}/connection/start`, { headers, data: {} })).json();
 expect((await page.request.post(`${base}/connection/callback`, { headers, data: { state, code: width === 360 ? "demo-page" : "other-page" } })).status()).toBe(200);
 try {
 let release!: () => void; const pending = new Promise<void>(resolve => { release = resolve; });
 await page.route(`**${base}/post-selections?*`, async route => { await pending; await route.continue(); });
 await page.goto("/dashboard/products");
 const extraction = page.getByRole("region", { name: "استخراج از کپشن" });
 await expect(extraction.getByRole("status")).toHaveText("در حال دریافت استخراج‌ها…");
 release();
 await expect(extraction.getByText("هنوز پستی برای استخراج انتخاب نشده است.", { exact: false })).toBeVisible();
 await page.unroute(`**${base}/post-selections?*`);
 const gallery = page.getByRole("region", { name: "مرور پست‌ها" });
 // Hold only the displayed status to inspect pending independently of worker speed.
 await page.route(`**${base}/post-selections?*`, async route => {
 const r = await route.fetch(); const data = await r.json();
 data.selections = data.selections.map((s: object) => ({ ...s, status: "pending", proposal: null, productId: null }));
 await route.fulfill({ response: r, json: data });
 });
 await gallery.getByRole("checkbox").first().check();
 await gallery.getByRole("button", { name: "ذخیرهٔ انتخاب‌ها" }).click();
 await expect(extraction.getByRole("heading", { name: "در انتظار استخراج" })).toBeVisible();
 await page.unroute(`**${base}/post-selections?*`);
 await extraction.getByRole("button", { name: "تازه‌سازی استخراج‌ها" }).focus();
 await page.keyboard.press("Enter");
 await expect(extraction.getByRole("heading", { name: "پیش‌نویس ساخته شد" })).toBeVisible({ timeout: 15000 });
 await expect(extraction.getByText("۸۹۰٬۰۰۰ تومان", { exact: true })).toBeVisible();
 await expect(extraction.getByText("نامعلوم؛ نیازمند تکمیل", { exact: true })).toBeVisible();
 await expect(extraction.getByText("از کپشن همین پست: «مانتو لینن نارون»")).toBeVisible();
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 expect(await page.locator("html").getAttribute("dir")).toBe("rtl");
 await page.evaluate(() => document.fonts.ready);
 expect(await page.evaluate(() => document.fonts.check("16px Vazirmatn"))).toBe(true);
 await extraction.screenshot({ path: `test-results/extraction-${width}.png` });
 const saved = await (await page.request.get(`${base}/post-selections`)).json();
 const selected = saved.selections[0];
 expect(selected.productId).toBeTruthy();
 expect((await page.request.post(`${base}/post-selections/${selected.selectionId}/retry`, { headers: { Origin: origin }, data: {} })).status()).toBe(403);
 await extraction.getByRole("button", { name: "بررسی و ویرایش محصول" }).click();
 await expect(page.getByRole("heading", { name: "جزئیات و ویرایش محصول" })).toBeFocused();
 await expect(page.getByLabel("نام محصول", { exact: true })).toHaveValue("مانتو لینن نارون");
 const editor = page.getByRole("region", { name: "فرم محصول" });
 const sources = editor.getByRole("region", { name: "شواهد کپشن" });
 await sources.locator("summary").click();
 await expect(sources).toContainText(selected.caption);
 await expect(sources).toContainText("از کپشن همین پست: «مانتو لینن نارون»");
 await page.getByLabel("نام تنوع", { exact: true }).fill("زیتونی متوسط");
 await expect(editor.getByRole("region", { name: "خلاصهٔ تغییرات" })).toContainText("زیتونی متوسط");
 await page.getByRole("button", { name: "ذخیره محصول", exact: true }).click();
 await expect(page.locator(".catalog [role=status]")).toHaveText("محصول ذخیره شد.");
 await expect(sources).toContainText(selected.caption);
 await editor.screenshot({ path: `test-results/approval-evidence-${width}.png` });
 await page.getByRole("button", { name: "تأیید صریح محصول ذخیره‌شده" }).click();
 await expect(page.getByRole("status").filter({ hasText: "محصول تأیید شد." })).toBeVisible();
 const detail = await (await page.request.get(`${base}/products/${selected.productId}`)).json();
 expect(detail.product.status).toBe("approved");
 expect(detail.product.variants[0].name).toBe("زیتونی متوسط");
 expect(detail.sources[0].caption).toBe(selected.caption);
 expect(detail.sources[0].proposal).toEqual(selected.proposal);
 // Failure/conflict display uses controlled responses; actual worker failures
 // and retry persistence are covered by the PostgreSQL API/worker tests.
 await page.route(`**${base}/post-selections?*`, async route => route.fulfill({ json: { selections: [{ ...selected, status: "failed", reason: "invalid_output", productId: null, proposal: null }], nextCursor: "" } }));
 await extraction.getByRole("button", { name: "تازه‌سازی استخراج‌ها" }).click();
 await expect(extraction.getByText("نتیجهٔ استخراج معتبر نبود؛ محصولی ساخته نشد.")).toBeVisible();
 await expect(extraction.getByRole("button", { name: "تکرار استخراج" })).toBeEnabled();
 await page.unroute(`**${base}/post-selections?*`);
 await page.route(`**${base}/post-selections?*`, async route => route.fulfill({ json: { selections: [{ ...selected, proposal: { ...selected.proposal, attributes: { رنگ: { state: "conflict", values: ["زیتونی", "کرم"], sources: [{ id: selected.selectionId, quote: "رنگ‌های زیتونی و کرم" }] } } } }], nextCursor: "" } }));
 await extraction.getByRole("button", { name: "تازه‌سازی استخراج‌ها" }).click();
 await expect(extraction.getByText("تعارض؛ نیازمند بررسی:", { exact: false })).toBeVisible();
 await page.unroute(`**${base}/post-selections?*`);
 await page.route(`**${base}/post-selections?*`, route => route.fulfill({ status: 503, json: { code: "posts_unavailable" } }));
 await extraction.getByRole("button", { name: "تازه‌سازی استخراج‌ها" }).click();
 await expect(extraction.getByRole("alert")).toContainText("وضعیت استخراج دریافت نشد");
 await page.unroute(`**${base}/post-selections?*`);
 await extraction.getByRole("button", { name: "تازه‌سازی استخراج‌ها" }).click();
 await expect(extraction.getByRole("alert")).toHaveCount(0);
 await page.reload();
 await expect(extraction.getByRole("heading", { name: "پیش‌نویس ساخته شد" })).toBeVisible();
 } finally {
 await page.request.post(`${base}/connection/disconnect`, { headers, data: {} });
 }
});
