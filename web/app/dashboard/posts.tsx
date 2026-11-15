"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import "./posts.css";
import Extractions from "./extractions";
type Post = { id: string; caption: string; publishedAt: string; selected: boolean; status: string };
const statuses: Record<string, string> = { pending: "در انتظار پردازش", processing: "در حال پردازش", completed: "پردازش‌شده", failed: "پردازش ناموفق", blocked: "نیازمند رسیدگی" };

export default function Posts({ store, csrf, onOpen }: { store: string; csrf: string; onOpen: (id: string) => void }) {
  const [refresh, setRefresh] = useState(0);
  const [posts, setPosts] = useState<Post[]>([]);
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [cursor, setCursor] = useState("");
  const [next, setNext] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [connection, setConnection] = useState(false);
  const lock = useRef(false);
  const retry = useRef<{ key: string; ids: string[] } | null>(null);
  const base = `/api/stores/${encodeURIComponent(store)}`;
  async function load(page = cursor) {
    setLoading(true); setError(""); setConnection(false);
    try {
      const r = await fetch(`${base}/posts?cursor=${encodeURIComponent(page)}`, { cache: "no-store" });
      if (r.status === 401) { window.location.replace("/login"); return; }
      const data = await r.json();
      if (!r.ok) { setConnection(data.code === "connection_required"); throw new Error(); }
      setPosts(data.posts); setCursor(page); setNext(data.nextCursor);
      setChosen(old => { const updated = new Set(old); data.posts.forEach((p: Post) => { if (p.selected) updated.delete(p.id); }); return updated; });
    } catch { setError("پست‌ها دریافت نشدند. اتصال صفحه را بررسی کنید یا دوباره تلاش کنید."); }
    finally { setLoading(false); }
  }
  useEffect(() => { void load(""); }, [store]); // eslint-disable-line react-hooks/exhaustive-deps
  async function save() {
    if (lock.current || !chosen.size) return;
    lock.current = true; setBusy(true); setError(""); setSuccess("");
    const ids = [...chosen].sort();
    if (!retry.current || JSON.stringify(retry.current.ids) !== JSON.stringify(ids)) retry.current = { key: crypto.randomUUID(), ids };
    try {
      const r = await fetch(`${base}/post-selections`, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ postIds: ids, key: retry.current.key }) });
      if (!r.ok) throw new Error();
      retry.current = null; setChosen(new Set()); setSuccess("انتخاب‌ها ذخیره شدند؛ در انتظار پردازش هستند. نتیجهٔ استخراج در پایین نمایش داده می‌شود.");
      await load(); setRefresh(n => n + 1);
    } catch { setError("انتخاب‌ها تأیید نشدند. برای بررسی یا تکرار امن درخواست، دوباره ذخیره کنید."); }
    finally { lock.current = false; setBusy(false); }
  }
  return <section className="post-browser" aria-label="مرور پست‌ها">
    <header><h2>پست‌های صفحه</h2><p>پست‌های معرفی محصول را انتخاب کنید. انتخاب پست قدیمی، پیام‌های قدیمی را وارد نمی‌کند.</p><small>داده‌های ساختگی برای نمایش؛ فقط متن کپشن برای استخراج استفاده می‌شود.</small></header>
    {success && <p className="post-success" role="status">{success}</p>}
    {error && <div role="alert" className="inline-error"><p>{error}</p>{connection && <Link href="/dashboard/connection">تنظیم اتصال صفحه</Link>} <button disabled={busy || loading} onClick={() => void load()}>تلاش دوباره</button></div>}
    {loading ? <p role="status">در حال دریافت پست‌ها…</p> : !error && posts.length === 0 ? <div className="empty-panel"><h3>پستی در این صفحه نیست</h3><p>پس از انتشار پست، فهرست را تازه کنید یا اتصال صفحه را بررسی کنید.</p><button onClick={() => void load("")}>تازه‌سازی فهرست</button><Link href="/dashboard/connection">تنظیم اتصال صفحه</Link></div> : !error && <div className="post-grid">{posts.map(p => <article key={p.id} className="post-card">
      <div className="post-caption-preview" aria-hidden="true">{p.caption.split("؛")[0]}</div>
      <div className="post-copy"><time dateTime={p.publishedAt}>{new Date(p.publishedAt).toLocaleDateString("fa-IR")}</time><p>{p.caption}</p>
      <label><input type="checkbox" checked={p.selected || chosen.has(p.id)} disabled={p.selected || busy} onChange={e => { const selected = e.target.checked; setSuccess(""); setChosen(old => { const copy = new Set(old); if (selected) copy.add(p.id); else copy.delete(p.id); return copy; }); }} />{p.selected ? "انتخاب ذخیره‌شده" : "انتخاب پست"}<span className="sr-only">: {p.caption}</span></label>
      {p.selected && <p className="post-state">{statuses[p.status] || "وضعیت نامشخص"}</p>}</div>
    </article>)}</div>}
    <div className="post-actions"><span aria-live="polite">{chosen.size.toLocaleString("fa-IR")} پست آمادهٔ ذخیره (همهٔ صفحات)</span><button disabled={!chosen.size || busy || loading || chosen.size > 20} onClick={() => void save()}>{busy ? "در حال ذخیره…" : "ذخیرهٔ انتخاب‌ها"}</button>{chosen.size > 20 && <p>در هر نوبت حداکثر ۲۰ پست انتخاب کنید.</p>}</div>
    <nav className="post-pagination" aria-label="صفحه‌های پست"><button disabled={!cursor || loading || busy} onClick={() => void load("")}>صفحهٔ اول</button><button disabled={!next || loading || busy} onClick={() => void load(next)}>پست‌های بعدی</button><button disabled={loading || busy} onClick={() => void load()}>تازه‌سازی وضعیت</button></nav>
    <Extractions store={store} csrf={csrf} refresh={refresh} onOpen={onOpen} />
  </section>;
}
