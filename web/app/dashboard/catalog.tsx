"use client";
import { useEffect, useRef, useState } from "react";
type Variant = { id: string; name: string; code: string | null; price: number | null; attributes: Record<string, string>; attributeRows?: [string, string][]; priceKnown?: boolean };
type Product = { id: string; name: string; revision: number; status: string; variants: Variant[] };
type History = { revision: number; changedAt: string; snapshot: Product };
const blankVariant = (): Variant => ({ id: "", name: "", code: null, price: null, attributes: {} });
const blank = (): Product => ({ id: "", name: "", revision: 0, status: "draft", variants: [blankVariant()] });
const statuses: Record<string, string> = { draft: "پیش‌نویس", approved: "تأییدشده", archived: "بایگانی‌شده" };
export default function Catalog({ store, csrf, openedProduct }: { store: string; csrf: string; openedProduct?: {id: string} | null }) {
  const base = `/api/stores/${encodeURIComponent(store)}/products`;
  const [items, setItems] = useState<Product[]>([]); const [product, setProduct] = useState<Product | null>(null);
  const [history, setHistory] = useState<History[]>([]);
  const [saved, setSaved] = useState("");
  const [query, setQuery] = useState(""); const [status, setStatus] = useState(""); const [cursor, setCursor] = useState(""); const [next, setNext] = useState("");
  const [loading, setLoading] = useState(true); const [busy, setBusy] = useState(false); const [error, setError] = useState(""); const [success, setSuccess] = useState("");
  const [fields, setFields] = useState<Record<string, string>>({});
  const lock = useRef(false); const generation = useRef(0);
  const editorHeading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { if (product) editorHeading.current?.focus(); }, [product?.id]);
  async function load(page = "") {
    const current = ++generation.current; setLoading(true); setError("");
    try {
      const response = await fetch(`${base}?${new URLSearchParams({ q: query, status, cursor: page })}`, { cache: "no-store" });
      if (response.status === 401) { window.location.replace("/login"); return; }
      if (!response.ok) throw new Error(); const data = await response.json();
      if (current === generation.current) { setItems(data.products); setNext(data.nextCursor); setCursor(page); }
    } catch { if (current === generation.current) setError("کاتالوگ دریافت نشد. دوباره تلاش کنید."); }
    finally { if (current === generation.current) setLoading(false); }
  }
  useEffect(() => { void load(); return () => { generation.current++; }; }, [store]);
  async function open(id: string) {
    if (lock.current) return; lock.current = true; setBusy(true); setError(""); setSuccess("");
    try { const response = await fetch(`${base}/${encodeURIComponent(id)}`, { cache: "no-store" }); if (!response.ok) throw new Error(); const data = await response.json(); setProduct(data.product); setSaved(JSON.stringify(data.product)); setHistory(data.history); setFields({}); }
    catch { setError("محصول دریافت نشد. دوباره تلاش کنید."); }
    finally { lock.current = false; setBusy(false); }
  }
  useEffect(() => { if (openedProduct) void open(openedProduct.id); }, [openedProduct]);
  async function mutate(action = "") {
    if (!product || lock.current) return; lock.current = true; setBusy(true); setError(""); setSuccess(""); setFields({});
    try {
      const variants = product.variants.map(({ attributeRows, priceKnown, ...v }) => {
        const rows = attributeRows ?? Object.entries(v.attributes);
        if (new Set(rows.map(([key]) => key)).size !== rows.length) throw new Error("duplicate_attributes");
        if (priceKnown && v.price === null) throw new Error("price_required");
        return { ...v, attributes: Object.fromEntries(rows) };
      });
      const body = action ? { revision: product.revision } : { name: product.name, variants, revision: product.revision };
      const response = await fetch(`${base}${product.id ? `/${encodeURIComponent(product.id)}` : ""}${action ? `/${action}` : ""}`, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) });
      const data = await response.json();
      if (!response.ok) { setFields(data.fieldErrors || {}); setError(data.code === "revision_conflict" ? "نسخهٔ محصول تغییر کرده است. با «بارگیری مجدد محصول» نسخهٔ تازه را بگیرید؛ تغییرات ذخیره‌نشده جایگزین می‌شوند." : data.code === "code_conflict" ? "این کد قبلاً استفاده شده است؛ کد دیگری وارد کنید." : "تغییرات ذخیره نشد. ورودی‌ها را بررسی و دوباره تلاش کنید."); return; }
      setProduct(data.product); setSaved(JSON.stringify(data.product)); setHistory([]); await load(); setSuccess(action === "approve" ? "محصول تأیید شد." : action === "archive" ? "محصول بایگانی شد؛ کدهای آن محفوظ می‌مانند." : "محصول ذخیره شد.");
    } catch (e) { setError(e instanceof Error && e.message === "duplicate_attributes" ? "نام ویژگی‌ها تکراری است؛ برای هر ویژگی نام جداگانه وارد کنید." : e instanceof Error && e.message === "price_required" ? "مبلغ را وارد کنید یا قیمت نامعلوم را انتخاب کنید." : "ارتباط برقرار نشد؛ ذخیره‌شدن را با بارگیری مجدد بررسی کنید."); }
    finally { lock.current = false; setBusy(false); }
  }
  function variant(index: number, patch: Partial<Variant>) { if (product) setProduct({ ...product, variants: product.variants.map((v, i) => i === index ? { ...v, ...patch } : v) }); }
  return <section className="catalog" aria-label="کاتالوگ محصولات">
    <div className="catalog-toolbar"><p>محصول‌ها و تنوع‌های فروشگاه، با قیمت معلوم یا نامعلوم.</p><button className="shop-action" disabled={busy} onClick={() => { setProduct(blank()); setHistory([]); setFields({}); setError(""); setSuccess(""); }}>ثبت محصول دستی</button></div>
    {error && <div role="alert" className="shop-error">{error}<button disabled={busy} onClick={() => void load(cursor)}>تلاش دوباره</button></div>}
    {success && <p role="status" className="shop-success">{success}</p>}
    {product && <section className="catalog-editor" aria-label="فرم محصول">
      <h2 ref={editorHeading} tabIndex={-1}>{product.id ? "جزئیات و ویرایش محصول" : "محصول دستی جدید"}</h2><p>{statuses[product.status]}{product.id && ` · نسخهٔ ${product.revision.toLocaleString("fa-IR")}`}</p>
      <form onSubmit={e => { e.preventDefault(); void mutate(); }}><fieldset disabled={busy || product.status === "archived"}>
        <label>نام محصول<input required maxLength={120} value={product.name} onChange={e => setProduct({ ...product, name: e.target.value })} /></label>
        {product.variants.map((v, i) => <fieldset className="catalog-variant" key={v.id || i}><legend>تنوع {(i + 1).toLocaleString("fa-IR")}</legend>
          <div className="catalog-fields"><label>نام تنوع<input required maxLength={120} value={v.name} onChange={e => variant(i, { name: e.target.value })} /></label>
          <label>کد اختیاری<input dir="auto" maxLength={80} value={v.code || ""} onChange={e => variant(i, { code: e.target.value || null })} /></label>
           <label>قیمت به تومان<input type="number" min="0" max="9007199254740991" step="1" required={v.priceKnown ?? v.price !== null} disabled={!(v.priceKnown ?? v.price !== null)} value={v.price ?? ""} onChange={e => variant(i, { price: e.target.value === "" ? null : Number(e.target.value) })} /></label>
           <label className="catalog-check"><input type="checkbox" checked={!(v.priceKnown ?? v.price !== null)} onChange={e => variant(i, { priceKnown: !e.target.checked, price: null })} />قیمت نامعلوم</label></div>
          <p>ویژگی‌ها (مانند رنگ و اندازه)</p>
           {(v.attributeRows ?? Object.entries(v.attributes)).map(([key, value], j) => <div className="catalog-attribute" key={j}><label>نام ویژگی<input required maxLength={80} value={key} onChange={e => variant(i, { attributeRows: (v.attributeRows ?? Object.entries(v.attributes)).map((row, n) => n === j ? [e.target.value, row[1]] : row) })} /></label><label>مقدار ویژگی<input required maxLength={200} value={value} onChange={e => variant(i, { attributeRows: (v.attributeRows ?? Object.entries(v.attributes)).map((row, n) => n === j ? [row[0], e.target.value] : row) })} /></label><button type="button" onClick={() => variant(i, { attributeRows: (v.attributeRows ?? Object.entries(v.attributes)).filter((_, n) => n !== j) })}>حذف ویژگی</button></div>)}
           <button type="button" disabled={(v.attributeRows ?? Object.entries(v.attributes)).length >= 20} onClick={() => variant(i, { attributeRows: [...(v.attributeRows ?? Object.entries(v.attributes)), ["", ""]] })}>افزودن ویژگی</button>
          {!v.id && product.variants.length > 1 && <button type="button" onClick={() => setProduct({ ...product, variants: product.variants.filter((_, n) => n !== i) })}>حذف تنوع جدید</button>}
        </fieldset>)}
        <button type="button" disabled={product.variants.length >= 20} onClick={() => setProduct({ ...product, variants: [...product.variants, blankVariant()] })}>افزودن تنوع</button>
        {Object.entries(fields).map(([key, value]) => <p className="shop-error" key={key}>{value}</p>)}
        <p>ذخیرهٔ محصول جدید آن را به پیش‌نویس تبدیل می‌کند. قیمت نامعلوم مانع تأیید کاتالوگ نیست.</p><button className="shop-action" type="submit">{busy ? "در حال ذخیره…" : "ذخیره محصول"}</button>
      </fieldset></form>
      <div className="catalog-actions">
        {product.id && product.status === "draft" && <button disabled={busy || JSON.stringify(product) !== saved} onClick={() => void mutate("approve")}>تأیید صریح محصول ذخیره‌شده</button>}
        {product.id && product.status !== "archived" && <button disabled={busy || JSON.stringify(product) !== saved} onClick={() => { if (window.confirm("این محصول بایگانی شود؟ کدهای آن قابل استفاده برای محصول دیگر نخواهند بود.")) void mutate("archive"); }}>بایگانی محصول</button>}
        {product.id && <button disabled={busy} onClick={() => void open(product.id)}>بارگیری مجدد محصول و تاریخچه</button>}<button disabled={busy} onClick={() => setProduct(null)}>بستن فرم</button>
      </div>
      {history.length > 0 && <details><summary>تاریخچهٔ تغییرات</summary><ol>{history.map(h => <li key={h.revision}>نسخهٔ {h.revision.toLocaleString("fa-IR")}، {new Date(h.changedAt).toLocaleString("fa-IR")}، {h.snapshot.name}، {statuses[h.snapshot.status]}<ul>{h.snapshot.variants.map(v => <li key={v.id}>{v.name}، {v.code || "بدون کد"}، {v.price === null ? "قیمت نامعلوم" : `${v.price.toLocaleString("fa-IR")} تومان`}</li>)}</ul></li>)}</ol></details>}
    </section>}
    <form className="catalog-filters" onSubmit={e => { e.preventDefault(); void load(); }}><label>جست‌وجوی نام یا کد<input maxLength={120} value={query} onChange={e => setQuery(e.target.value)} /></label><label>وضعیت<select value={status} onChange={e => setStatus(e.target.value)}><option value="">همهٔ وضعیت‌ها</option>{Object.entries(statuses).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label><button disabled={loading || busy}>جست‌وجو</button></form>
    {loading ? <p role="status">در حال دریافت محصولات…</p> : items.length === 0 ? <div className="empty-work"><h2>محصولی پیدا نشد</h2><p>محصول دستی ثبت کنید یا فیلترهای جست‌وجو را تغییر دهید.</p></div> : <ul className="catalog-list">{items.map(p => <li key={p.id}><div className="catalog-title"><h2>{p.name || "نام محصول نامعلوم"}</h2><span>{statuses[p.status]}</span><button disabled={busy} onClick={() => void open(p.id)}>مشاهده و ویرایش</button></div><ul>{p.variants.map(v => <li key={v.id}><strong>{v.name}</strong><span>{v.code ? <bdi>{v.code}</bdi> : "بدون کد"}</span><span>{Object.entries(v.attributes).map(([k, value]) => `${k}: ${value}`).join("، ")}</span><span className="catalog-price">{v.price === null ? "قیمت نامعلوم" : `${v.price.toLocaleString("fa-IR")} تومان`}</span></li>)}</ul></li>)}</ul>}
    <div className="catalog-actions">{cursor && <button disabled={loading || busy} onClick={() => void load()}>صفحهٔ نخست</button>}{next && <button disabled={loading || busy} onClick={() => void load(next)}>صفحهٔ بعد</button>}</div>
  </section>;
}
