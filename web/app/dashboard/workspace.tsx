"use client";
import Link from "next/link";
import { useEffect, useRef, useState, type FormEvent } from "react";
import "./workspace.css";

import { sections, type Section } from "./sections";
import ConnectionSettings from "./connection";
import Processing from "./processing";
import Catalog from "./catalog";
import Posts from "./posts";
type Me = { user: { username: string }; stores: { id: string; name: string }[]; csrfToken: string };

export default function Dashboard({ section }: { section: Section }) {
  const [openedProduct, setOpenedProduct] = useState<{id: string} | null>(null);
  const [me, setMe] = useState<Me | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [created, setCreated] = useState(false);
  const [name, setName] = useState("");
  const [menu, setMenu] = useState(false);
  const locked = useRef(false);
  const heading = useRef<HTMLHeadingElement>(null);
  async function load() {
    setLoading(true); setError("");
    try {
      const response = await fetch("/api/me", { cache: "no-store" });
      if (response.status === 401) { window.location.replace("/login"); return; }
      if (!response.ok) throw new Error();
      setMe(await response.json());
    } catch { setError("اطلاعات فروشگاه دریافت نشد. دوباره تلاش کنید."); }
    finally { setLoading(false); }
  }
  useEffect(() => { void load(); }, []);
  useEffect(() => { if (created) heading.current?.focus(); }, [created]);
  async function create(event: FormEvent) {
    event.preventDefault(); if (!me || locked.current) return;
    locked.current = true; setBusy(true); setError("");
    try {
      const response = await fetch("/api/stores", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": me.csrfToken }, body: JSON.stringify({ name }) });
      if (response.status === 401) { window.location.replace("/login"); return; }
      const result = await response.json();
      if (response.status === 409) { await load(); return; }
      if (!response.ok) { setError(result.fieldErrors?.name || "فروشگاه ساخته نشد. دوباره تلاش کنید؛ اگر درخواست قبلی ثبت شده باشد، فروشگاه دوم ساخته نمی‌شود."); return; }
      setMe({ ...me, stores: [result.store] }); setCreated(true);
    } catch { setError("نتیجهٔ ساخت مشخص نیست. دوباره تلاش کنید؛ فروشگاه تکراری ساخته نمی‌شود."); }
    finally { locked.current = false; setBusy(false); }
  }
  async function logout() {
    if (!me || locked.current) return;
    locked.current = true; setBusy(true); setError("");
    try {
      const response = await fetch("/api/auth/logout", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": me.csrfToken }, body: "{}" });
      if (response.ok || response.status === 401) { window.location.replace("/login"); return; }
      throw new Error();
    } catch { setError("خروج انجام نشد؛ دوباره تلاش کنید."); }
    finally { locked.current = false; setBusy(false); }
  }
  const store = me?.stores[0];
  const content = sections[section];
  return <div className="workspace">
    <a className="skip-work" href="#work">رفتن به محتوای اصلی</a>
    <aside className="shop-sidebar">
      <div className="shop-brand">دستیار فروشگاه</div>
      <p className="shop-name">{store?.name || "راه‌اندازی فروشگاه"}</p>
      <button className="mobile-menu" aria-expanded={menu} aria-controls="shop-navigation" onClick={() => setMenu(!menu)}>منوی فروشگاه</button>
      <nav id="shop-navigation" aria-label="بخش‌های فروشگاه" className={menu ? "is-open" : ""}>
        {(Object.keys(sections) as Section[]).map(key => <Link key={key} href={key === "orders" ? "/dashboard" : `/dashboard/${key}`} aria-current={key === section ? "page" : undefined} onClick={() => setMenu(false)}>{sections[key].title}</Link>)}
      </nav>
      {me && <div className="shop-account"><span dir="auto">{me.user.username}</span><button disabled={busy} onClick={logout}>{busy ? "لطفاً صبر کنید…" : "خروج"}</button></div>}
    </aside>
    <main id="work" className="shop-work" aria-busy={loading}>
      <header className="work-header"><h1 ref={heading} tabIndex={-1}>{store ? content.title : "ساخت فروشگاه"}</h1>{store && section !== "connection" && <Link className="shop-action" href="/dashboard/connection">بررسی اتصال پیج</Link>}</header>
      {error && <div role="alert" className="shop-error">{error}{!me && <button onClick={load}>تلاش دوباره</button>}</div>}
      {loading ? <p role="status" className="shop-loading">در حال دریافت اطلاعات فروشگاه…</p> : !me ? null : !store ? <section className="setup-panel">
        <h2>فروشگاهتان را نام‌گذاری کنید</h2><p>این نام در پنل شما نمایش داده می‌شود. هر حساب فعلاً می‌تواند مدیر یک فروشگاه باشد.</p>
        <form onSubmit={create}><label htmlFor="store-name">نام فروشگاه</label><input autoComplete="organization" id="store-name" name="name" required maxLength={80} value={name} onChange={e => setName(e.target.value)} disabled={busy} aria-describedby="store-help" /><p id="store-help">برای نمونه: پوشاک نارون</p><button className="shop-action" disabled={busy || !name.trim()}>{busy ? "در حال ساخت فروشگاه…" : "ساخت فروشگاه"}</button></form>
      </section> : <>
        {created && <p role="status" className="shop-success">فروشگاه «{store.name}» ساخته شد. شما مدیر این فروشگاه هستید.</p>}
        {section === "products" ? <><Posts store={store.id} csrf={me.csrfToken} onOpen={id => setOpenedProduct({id})} /><Catalog store={store.id} csrf={me.csrfToken} openedProduct={openedProduct} /></> : section === "processing" ? <Processing store={store.id} csrf={me.csrfToken} /> : section === "connection" ? <ConnectionSettings store={store.id} csrf={me.csrfToken} /> : <section className="empty-work"><div className="empty-mark" aria-hidden="true">▤</div><h2>{content.empty}</h2><p>{content.description}</p>
          <Link className="shop-action" href="/dashboard/connection">رفتن به تنظیمات اتصال</Link>
        </section>}
      </>}
    </main>
  </div>;
}
