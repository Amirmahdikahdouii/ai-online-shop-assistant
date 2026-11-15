"use client";
import { useEffect, useRef, useState } from "react";

type Field = { state: "known" | "unknown" | "conflict"; values: string[]; sources: { id: string; quote: string }[] };
export type Proposal = { outcome: string; name: Field; code: Field; price: Field; attributes: Record<string, Field> };
type Selection = { selectionId: string; caption: string; publishedAt: string; selectedAt: string; status: string; reason: string; attempts: number; productId: string | null; proposal: Proposal | null };
const states: Record<string, string> = { pending: "در انتظار استخراج", processing: "در حال استخراج", completed: "پیش‌نویس ساخته شد", failed: "استخراج ناموفق", blocked: "نیازمند رسیدگی" };
const reasons: Record<string, string> = { timeout: "زمان استخراج تمام شد.", invalid_output: "نتیجهٔ استخراج معتبر نبود؛ محصولی ساخته نشد.", refused: "استخراج‌کننده پاسخی ارائه نکرد.", unavailable: "سرویس استخراج در دسترس نیست؛ پس از رفع مشکل دوباره تلاش کنید.", provider_error: "ارتباط با سرویس استخراج برقرار نشد.", no_product: "محصول مشخصی استخراج نشد؛ کپشن را بررسی کنید یا محصول را دستی ثبت کنید." };
export function EvidenceField({ label, field, money = false }: { label: string; field: Field; money?: boolean }) {
 return <div className={`extraction-field field-${field.state}`}><dt>{label}</dt><dd>
 {field.state === "unknown" ? <span className="field-missing">نامعلوم؛ نیازمند تکمیل</span> : <>{field.state === "conflict" && <strong className="field-conflict">تعارض؛ نیازمند بررسی: </strong>}{field.values.map((value, i) => <span key={i}>{i > 0 && " / "}{money ? `${Number(value).toLocaleString("fa-IR")} تومان` : value}</span>)}</>}
 {field.sources.map((source, i) => <small className="field-evidence" key={i}>از کپشن همین پست: «{source.quote}»</small>)}
 </dd></div>;
}
export default function Extractions({ store, csrf, refresh, onOpen }: { store: string; csrf: string; refresh: number; onOpen: (id: string) => void }) {
 const [items, setItems] = useState<Selection[]>([]), [loading, setLoading] = useState(true), [error, setError] = useState("");
 const [cursor, setCursor] = useState(""), [next, setNext] = useState(""), [busy, setBusy] = useState("");
 const base = `/api/stores/${encodeURIComponent(store)}/post-selections`;
 const generation = useRef(0), lock = useRef(false);
 async function load(page = cursor, silent = false) {
 const current = ++generation.current;
 if (!silent) { setLoading(true); setError(""); }
 try {
 const r = await fetch(`${base}?cursor=${encodeURIComponent(page)}`, { cache: "no-store" });
 if (r.status === 401) { window.location.replace("/login"); return; }
 if (!r.ok) throw new Error(); const data = await r.json();
 if (current === generation.current) { setItems(data.selections); setNext(data.nextCursor); setCursor(page); setError(""); }
 } catch { if (current === generation.current) setError("وضعیت استخراج دریافت نشد. دوباره تلاش کنید."); }
 finally { if (current === generation.current) setLoading(false); }
 }
 useEffect(() => { void load(""); return () => { generation.current++; }; }, [store, refresh]); // eslint-disable-line react-hooks/exhaustive-deps
 useEffect(() => {
 if (!items.some(s => s.status === "pending" || s.status === "processing") || error) return;
 const timer = setTimeout(() => void load(cursor, true), 2000); return () => clearTimeout(timer);
 }, [items, error, cursor]); // eslint-disable-line react-hooks/exhaustive-deps
 async function retry(id: string) {
 if (lock.current) return; lock.current = true; setBusy(id); setError("");
 try { const r = await fetch(`${base}/${encodeURIComponent(id)}/retry`, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: "{}" }); if (!r.ok) throw new Error(); await load(); }
 catch { setError("درخواست تکرار تأیید نشد. وضعیت را تازه کنید و دوباره تلاش کنید."); }
 finally { lock.current = false; setBusy(""); }
 }
 return <section className="extractions" aria-label="استخراج از کپشن">
 <header><h2>از کپشن تا پیش‌نویس</h2><p>اطلاعات پیشنهادی را با متن پست مقایسه کنید. محصول تازه تا تأیید صریح شما پیش‌نویس می‌ماند.</p><p>تصویر پست تحلیل نمی‌شود. در دمو، استخراج با پاسخ‌های قطعی شبیه‌سازی می‌شود.</p></header>
 {error && <p role="alert" className="inline-error">{error}</p>}
 <button disabled={loading || !!busy} onClick={() => void load()}>تازه‌سازی استخراج‌ها</button>
 {loading ? <p role="status">در حال دریافت استخراج‌ها…</p> : items.length === 0 ? <p>هنوز پستی برای استخراج انتخاب نشده است. از فهرست بالا پست معرفی محصول را انتخاب و ذخیره کنید.</p> : items.map(s => <article className="extraction-comparison" key={s.selectionId}>
 <div className="extraction-caption"><h3>کپشن منتخب</h3><time dateTime={s.publishedAt}>{new Date(s.publishedAt).toLocaleDateString("fa-IR")}</time><blockquote>{s.caption || "کپشن خالی است."}</blockquote></div>
 <div className="extraction-result"><h3>{states[s.status] || "وضعیت نامشخص"}</h3>
 {(s.status === "pending" || s.status === "processing") && <p>کار ذخیره شده است؛ پس از پایان پردازش، نتیجه اینجا نمایش داده می‌شود.</p>}
 {(s.status === "failed" || s.status === "blocked") && <><p className="field-conflict">{reasons[s.reason] || "استخراج به بررسی نیاز دارد."}</p><button disabled={!!busy} onClick={() => void retry(s.selectionId)}>{busy === s.selectionId ? "در حال ثبت…" : "تکرار استخراج"}</button></>}
 {s.proposal?.outcome === "product" && <><dl><EvidenceField label="نام محصول" field={s.proposal.name} /><EvidenceField label="کد پیشنهادی (برای ثبت، بررسی کنید)" field={s.proposal.code} /><EvidenceField label="قیمت واحد" field={s.proposal.price} money />{Object.entries(s.proposal.attributes).map(([key, field]) => <EvidenceField key={key} label={key} field={field} />)}</dl><p>این نتیجهٔ اولیهٔ استخراج است؛ تغییرات بعدی شما در فرم محصول ثبت می‌شوند.</p></>}
 {s.productId && <button onClick={() => onOpen(s.productId!)}>بررسی و ویرایش محصول</button>}
 </div></article>)}
 <nav className="post-pagination" aria-label="صفحه‌های استخراج"><button disabled={!cursor || loading || !!busy} onClick={() => void load("")}>نخستین استخراج‌ها</button><button disabled={!next || loading || !!busy} onClick={() => void load(next)}>استخراج‌های بعدی</button></nav>
 </section>;
}
