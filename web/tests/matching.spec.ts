import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";

for (const width of [360, 1280]) test(`auto matching presentation, evidence and review at ${width}px`, async ({ page }) => {
  await page.setViewportSize({ width, height: 960 });
  await page.goto("/login");
  const origin = new URL(page.url()).origin;
  const credentials = { username: `match_${randomUUID().replaceAll("-", "").slice(0, 12)}`, password: "matching-password-123" };
  expect((await page.request.post("/api/auth/register", { data: credentials })).status()).toBe(201);
  expect((await page.request.post("/api/auth/login", { headers: { Origin: origin }, data: credentials })).status()).toBe(200);
  const me = await (await page.request.get("/api/me")).json();
  const headers = { Origin: origin, "X-CSRF-Token": me.csrfToken };
  const { store } = await (await page.request.post("/api/stores", { headers, data: { name: "پوشاک نارون" } })).json();
  const base = `/api/stores/${store.id}`;
  const { state } = await (await page.request.post(`${base}/connection/start`, { headers, data: {} })).json();
  expect((await page.request.post(`${base}/connection/callback`, { headers, data: { state, code: width === 360 ? "demo-page" : "other-page" } })).status()).toBe(200);

  // Create and approve a catalog product with code LIN-01
  const prodRes = await (await page.request.post(`${base}/products`, {
    headers,
    data: { name: "پیراهن لینن نورا", variants: [{ name: "آبی متوسط", code: "LIN-01", price: 520000, attributes: { مدل: "خنک و تابستانی" } }] }
  })).json();
  expect((await page.request.post(`${base}/products/${prodRes.product.id}/approve`, { headers, data: { revision: 1 } })).status()).toBe(200);

  // Route post-selections to verify presentation with real contract fixtures
  await page.route(`**${base}/post-selections?*`, async route => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        selections: [
          {
            selectionId: "sel-matched-01",
            id: "post-matched-01",
            caption: "پیراهن لینن نورا؛ کد LIN-01، خنک و تابستانی. قیمت ۵۲۰٬۰۰۰ تومان.",
            publishedAt: "2026-11-15T10:00:00Z",
            selectedAt: "2026-11-15T10:05:00Z",
            status: "completed",
            reason: "product",
            attempts: 1,
            productId: null,
            proposal: {
              outcome: "product",
              name: { state: "known", values: ["پیراهن لینن نورا"], sources: [{ id: "sel-matched-01", quote: "پیراهن لینن نورا" }] },
              code: { state: "known", values: ["LIN-01"], sources: [{ id: "sel-matched-01", quote: "LIN-01" }] },
              price: { state: "known", values: ["520000"], sources: [{ id: "sel-matched-01", quote: "قیمت ۵۲۰٬۰۰۰ تومان" }] },
              attributes: { مدل: { state: "known", values: ["خنک و تابستانی"], sources: [{ id: "sel-matched-01", quote: "خنک و تابستانی" }] } }
            },
            matchStatus: "matched",
            matchEvidence: {
              reason: "explicit_code",
              summary: "تطبیق خودکار با کد صریح «LIN-01»",
              policy: "caption-v1",
              model: "scripted-v1",
              quotes: ["LIN-01"],
              candidates: [
                { productId: prodRes.product.id, productName: "پیراهن لینن نورا", variantId: prodRes.product.variants[0].id, variantName: "آبی متوسط", code: "LIN-01" }
              ]
            },
            mappingRevision: 1,
            mappingTargets: [{ productId: prodRes.product.id, variantId: prodRes.product.variants[0].id }]
          },
          {
            selectionId: "sel-unresolved-01",
            id: "post-unresolved-01",
            caption: "کت جین پاییزه؛ کد UNK-99، تک‌رنگ آبی تیره. قیمت ۹۸۰٬۰۰۰ تومان.",
            publishedAt: "2026-11-15T11:00:00Z",
            selectedAt: "2026-11-15T11:05:00Z",
            status: "completed",
            reason: "product",
            attempts: 1,
            productId: null,
            proposal: {
              outcome: "product",
              name: { state: "known", values: ["کت جین پاییزه"], sources: [{ id: "sel-unresolved-01", quote: "کت جین پاییزه" }] },
              code: { state: "known", values: ["UNK-99"], sources: [{ id: "sel-unresolved-01", quote: "UNK-99" }] },
              price: { state: "known", values: ["980000"], sources: [{ id: "sel-unresolved-01", quote: "قیمت ۹۸۰٬۰۰۰ تومان" }] },
              attributes: {}
            },
            matchStatus: "unresolved",
            matchEvidence: {
              reason: "unknown_code",
              summary: "کد «UNK-99» در کاتالوگ فروشگاه یافت نشد.",
              policy: "caption-v1",
              model: "scripted-v1",
              quotes: ["UNK-99"],
              candidates: []
            },
            mappingRevision: 0,
            mappingTargets: []
          }
        ],
        nextCursor: ""
      })
    });
  });

  await page.goto("/dashboard/products");
  const extraction = page.getByRole("region", { name: "استخراج از کپشن" });

  // 1. Matched post verification
  const matchedArticle = extraction.locator("article").filter({ hasText: "sel-matched-01" }).or(extraction.locator("article").filter({ hasText: "LIN-01" })).first();
  await expect(matchedArticle.locator(".match-status-matched")).toHaveText("تطبیق شد");
  await expect(matchedArticle).toContainText("تطبیق خودکار با کد صریح «LIN-01»");

  // 2. Unresolved post verification
  const unresolvedArticle = extraction.locator("article").filter({ hasText: "UNK-99" }).first();
  await expect(unresolvedArticle.locator(".match-status-unresolved")).toHaveText("نیاز به بررسی");
  await expect(unresolvedArticle).toContainText("کد «UNK-99» در کاتالوگ فروشگاه یافت نشد.");

  // 3. Expandable evidence interaction via keyboard/focus
  const summaryToggle = matchedArticle.locator("details.match-details summary");
  await summaryToggle.focus();
  await page.keyboard.press("Enter");
  await expect(matchedArticle.locator(".match-details-content")).toBeVisible();
  await expect(matchedArticle.locator(".match-details-content")).toContainText("«LIN-01»");
  await expect(matchedArticle.locator(".match-details-content")).toContainText("نسخهٔ سیاست: caption-v1");
  await expect(matchedArticle.locator(".match-details-content")).toContainText("مدل: scripted-v1");

  // Verify NO misleading confidence percentage
  const matchedText = await matchedArticle.innerText();
  expect(matchedText).not.toMatch(/\d+%/);
  expect(matchedText).not.toContain("درصد اطمینان");

  // 4. Layout, RTL and Screenshot
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(await page.locator("html").getAttribute("dir")).toBe("rtl");
  await page.evaluate(() => document.fonts.ready);
  await extraction.screenshot({ path: `test-results/matching-${width}.png` });

  await page.unroute(`**${base}/post-selections?*`);
  expect((await page.request.post(`${base}/connection/disconnect`, { headers, data: {} })).status()).toBe(200);
});
