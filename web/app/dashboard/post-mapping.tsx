"use client";
import { useEffect, useRef, useState } from "react";
import type { Product } from "./catalog-review";
type Target = { productId: string; variantId: string };
type Mapping = { revision: number; notProduct: boolean; targets: Target[] };
type Change = { revision: number; actorId: string; source: string; changedAt: string; snapshot: Mapping };
const same = (a: Target, b: Target) => a.productId === b.productId && a.variantId === b.variantId;
export default function PostMapping({ store, selection, csrf }: { store: string; selection: string; csrf: string }) {
 const [open, setOpen] = useState(false), [mapping, setMapping] = useState<Mapping | null>(null), [saved, setSaved] = useState<Mapping | null>(null);
 const [history, setHistory] = useState<Change[]>([]), [products, setProducts] = useState<Product[]>([]), [known, setKnown] = useState<Record<string, Product>>({});
 const [query, setQuery] = useState(""), [next, setNext] = useState(""), [loading, setLoading] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState(""), [success, setSuccess] = useState("");
 const lock = useRef(false), generation = useRef(0), heading = useRef<HTMLHeadingElement>(null);
 const base = `/api/stores/${encodeURIComponent(store)}`, url = `${base}/post-selections/${encodeURIComponent(selection)}/mapping`;
 const remember = (items: Product[]) => setKnown(old => ({ ...old, ...Object.fromEntries(items.map(p => [p.id, p])) }));
 async function search(cursor = "") {
 const current = ++generation.current; setLoading(true); setError("");
 try { const r = await fetch(`${base}/products?${new URLSearchParams({ q: query, cursor })}`, { cache: "no-store" }); if (!r.ok) throw new Error(); const d = await r.json(); if (current === generation.current) { setProducts(d.products.filter((p: Product) => p.status !== "archived")); remember(d.products); setNext(d.nextCursor); } }
 catch { if (current === generation.current) setError("محصول‌ها دریافت نشدند؛ جست‌وجو را دوباره انجام دهید."); }
 finally { if (current === generation.current) setLoading(false); }
 }
 async function load() {
 if (lock.current) return; lock.current = true; setBusy(true); setError("");
 try {
 const r = await fetch(url, { cache: "no-store" }); if (!r.ok) throw new Error(); const d = await r.json();
 const ids = [...new Set<string>(d.mapping.targets.map((t: Target) => t.productId))];
 const details = await Promise.all(ids.map(async id => { const response = await fetch(`${base}/products/${encodeURIComponent(id)}`, { cache: "no-store" }); if (!response.ok) throw new Error(); return (await response.json()).product as Product; }));
 remember(details); setMapping(d.mapping); setSaved(d.mapping); setHistory(d.history);
 } catch { setError("ارتباط ذخیره‌شده دریافت نشد؛ بارگیری مجدد را بزنید."); }
 finally { lock.current = false; setBusy(false); }
 }
 useEffect(() => { if (open) { heading.current?.focus(); void load(); void search(); } return () => { generation.current++; }; }, [open, store, selection]);
 function toggle(target: Target, checked: boolean) {
 if (!mapping) return;
 const targets = mapping.targets.filter(t => !same(t, target) && !(checked && t.productId === target.productId && (!t.variantId || !target.variantId)));
 if (checked) targets.push(target);
 setMapping({ ...mapping, notProduct: false, targets }); setSuccess("");
 }
 async function save() {
 if (!mapping || lock.current) return; lock.current = true; setBusy(true); setError(""); setSuccess("");
 try {
 const r = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(mapping) }); const d = await r.json();
 if (!r.ok) { setError(d.code === "revision_conflict" ? "ارتباط توسط مدیر تغییر کرده است. بارگیری مجدد را بزنید و انتخاب‌ها را بررسی کنید." : "ارتباط ذخیره نشد؛ محصول و تنوع معتبر همین فروشگاه را انتخاب کنید."); return; }
 setMapping(d.mapping); setSaved(d.mapping); setSuccess("ارتباط پست ذخیره شد.");
 await search();
 // The save has committed; history refresh failure must not imply save failure.
 try { const detail = await fetch(url, { cache: "no-store" }); if (!detail.ok) throw new Error(); setHistory((await detail.json()).history); } catch { setError("ارتباط ذخیره شد؛ تاریخچه دریافت نشد. بارگیری مجدد را بزنید."); }
 } catch { setError("پاسخ ذخیره دریافت نشد؛ دوباره ذخیره کنید یا بارگیری مجدد را بزنید."); }
 finally { lock.current = false; setBusy(false); }
 }
 const label = (t: Target) => { const p = known[t.productId], v = p?.variants.find(v => v.id === t.variantId); return p ? `${p.name} — ${v ? `${v.name}، ${v.code || "بدون کد"}` : "محصول، بدون تعیین تنوع"}${p.status === "draft" ? " (پیش‌نویس)" : p.status === "archived" ? " (بایگانی‌شده)" : ""}` : t.productId; };
 return <section className="post-mapping" aria-label="ارتباط پست با محصول">
 <button onClick={() => setOpen(!open)} aria-expanded={open}>{open ? "بستن ارتباط محصولات" : "انتخاب یا اصلاح ارتباط محصولات"}</button>
 {open && <><h3 ref={heading} tabIndex={-1}>محصول‌های این پست</h3><p>چند انتخاب، نامزدهای این پست را مشخص می‌کند؛ به معنی سفارش همهٔ اقلام نیست. محصول پیش‌نویس تا تأیید، قلم قطعی سفارش نمی‌شود.</p><p>با انتخاب محصول دیگری یا «این پست محصول نیست»، پیش‌نویس استخراج‌شدهٔ بدون ویرایش بایگانی می‌شود؛ محصولی که ویرایش کرده‌اید حفظ می‌شود.</p>
 {error && <p role="alert" className="inline-error">{error}</p>}{success && <p role="status" className="post-success">{success}</p>}
 <button disabled={busy} onClick={() => { void load(); }}>بارگیری مجدد ارتباط و تاریخچه</button>
 {busy && <p role="status">در حال دریافت یا ذخیرهٔ ارتباط…</p>}
 {saved && <div className="mapping-saved"><strong>نتیجهٔ ذخیره‌شده، نسخهٔ {saved.revision.toLocaleString("fa-IR")}</strong>{!saved.revision ? <p>هنوز تصمیمی ثبت نشده است.</p> : saved.notProduct ? <p>این پست محصول نیست</p> : <ul>{saved.targets.map(t => <li key={`${t.productId}:${t.variantId}`}>{label(t)}</li>)}</ul>}</div>}
 {mapping && <><fieldset disabled={busy}><legend>انتخاب ارتباط</legend>
 <label className="mapping-choice"><input type="checkbox" checked={mapping.notProduct} onChange={e => { setMapping({ ...mapping, notProduct: e.target.checked, targets: [] }); setSuccess(""); }} /><span>این پست محصول نیست</span></label>
 <p>انتخاب‌های فعلی</p>{mapping.targets.length ? mapping.targets.map(t => <label className="mapping-choice" key={`${t.productId}:${t.variantId}`}><input type="checkbox" checked onChange={() => toggle(t, false)} /><span>{label(t)}</span></label>) : <p>محصولی انتخاب نشده است.</p>}
 </fieldset>
 <form className="mapping-search" onSubmit={e => { e.preventDefault(); void search(); }}><label>جست‌وجوی محصول، کد یا تنوع<input maxLength={120} value={query} onChange={e => setQuery(e.target.value)} /></label><button disabled={loading || busy}>جست‌وجوی محصولات</button></form>
 {loading ? <p role="status">در حال جست‌وجوی محصولات…</p> : products.length === 0 ? <p>محصولی پیدا نشد؛ عبارت جست‌وجو را تغییر دهید یا محصولی در کاتالوگ ثبت کنید.</p> : <fieldset disabled={busy}><legend>محصول‌ها و تنوع‌های همین فروشگاه</legend>{products.map(p => <div className="mapping-product" key={p.id}><label className="mapping-choice"><input type="checkbox" checked={mapping.targets.some(t => same(t, { productId: p.id, variantId: "" }))} onChange={e => toggle({ productId: p.id, variantId: "" }, e.target.checked)} /><span>{p.name}، بدون تعیین تنوع {p.status === "draft" && "(پیش‌نویس)"}</span></label>{p.variants.map(v => <label className="mapping-choice" key={v.id}><input type="checkbox" checked={mapping.targets.some(t => same(t, { productId: p.id, variantId: v.id }))} onChange={e => toggle({ productId: p.id, variantId: v.id }, e.target.checked)} /><span>{p.name}، {v.name}، <bdi>{v.code || "بدون کد"}</bdi></span></label>)}</div>)}</fieldset>}
 {next && <button disabled={loading || busy} onClick={() => void search(next)}>محصول‌های بعدی</button>}
 <button className="shop-action" disabled={busy || loading || (!mapping.notProduct && !mapping.targets.length)} onClick={() => void save()}>ذخیرهٔ ارتباط پست</button>
 </>}
 {history.length > 0 && <details><summary>تاریخچهٔ ارتباط پست</summary><ol>{history.map(h => <li key={h.revision}>نسخهٔ {h.revision.toLocaleString("fa-IR")}، اصلاح مدیر، {new Date(h.changedAt).toLocaleString("fa-IR")}، مدیر: <bdi>{h.actorId}</bdi>؛ {h.snapshot.notProduct ? "این پست محصول نیست" : `${h.snapshot.targets.length.toLocaleString("fa-IR")} ارتباط محصول/تنوع`}</li>)}</ol></details>}
 </>}
 </section>;
}
