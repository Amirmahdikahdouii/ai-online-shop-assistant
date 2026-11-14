"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";

type Item = { id: string; receivedAt: string; speaker: string; status: string; reason: string; attempts: number };
const statuses: Record<string, string> = { pending: "در انتظار پردازش", processing: "در حال پردازش", succeeded: "پردازش موفق", failed: "پردازش ناموفق" };
const reasons: Record<string, string> = { queued: "پیام ذخیره شد و در صف قرار گرفت.", working: "پیام دریافت شده؛ کار هنوز تمام نشده است.", processed: "بررسی نمونه تمام شد؛ سفارش یا پاسخ خودکار ساخته نمی‌شود.", sample_failure: "خطای کنترل‌شدهٔ نمونه؛ پس از سه تلاش متوقف می‌شود." };

export default function Processing({ store, csrf }: { store: string; csrf: string }) {
  const [data, setData] = useState<{ items: Item[]; fakeAvailable: boolean } | null>(null);
  const [error, setError] = useState("");
  const [submitError, setSubmitError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const last = useRef<{ scenario: string; key: string } | null>(null);
  const locked = useRef(false);
  const base = `/api/stores/${encodeURIComponent(store)}`;
  async function refresh() {
    const r = await fetch(`${base}/processing`, { cache: "no-store" });
    if (r.status === 401) { window.location.replace("/login"); return; }
    if (!r.ok) throw new Error();
    setData(await r.json()); setError("");
  }
  useEffect(() => {
    let active = true;
    const load = () => { if (active) void refresh().catch(() => { if (active) setError("وضعیت پیام‌ها دریافت نشد. دوباره تلاش کنید."); }); };
    load(); const timer = setInterval(load, 2000);
    return () => { active = false; clearInterval(timer); };
  }, [store]); // eslint-disable-line react-hooks/exhaustive-deps
  async function send(scenario: string, repeat = false) {
    if (locked.current) return;
    locked.current = true; setBusy(true); setSubmitError(""); setNotice("");
    if (!repeat || !last.current) last.current = { scenario, key: crypto.randomUUID() };
    try {
      const r = await fetch(`${base}/sample-events`, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(last.current) });
      if (r.status === 401) { window.location.replace("/login"); return; }
      const result = await r.json();
      if (!r.ok) throw new Error(result.code);
      setNotice(result.result === "duplicate" ? "این رویداد قبلاً ثبت شده؛ کار دوم ساخته نشد." : result.result === "ignored" ? "پیام مربوط به پیش از شروع اتصال بود و کنار گذاشته شد." : "پیام ذخیره و وارد صف شد؛ این تأیید به معنی پایان پردازش نیست.");
      await refresh();
    } catch (e) { setSubmitError(e instanceof Error && e.message === "connection_required" ? "ابتدا پیج آزمایشی را در تنظیمات اتصال متصل کنید." : "دریافت تأیید نشد؛ وضعیت را بررسی کنید. تکرار آخرین پیام، کار دوم نمی‌سازد."); }
    finally { locked.current = false; setBusy(false); }
  }
  return <section className="message-ledger" aria-label="دریافت و پردازش پیام">
    <h2>از دریافت تا پردازش</h2><p>آخرین ۱۰۰ پیام، تازه‌ترین در ابتدا. وضعیت هر دو ثانیه از فروشگاه شما خوانده می‌شود.</p>
    {data?.fakeAvailable && <div className="sample-controls"><p>نمونهٔ شبیه‌سازی‌شده؛ بدون اتصال به سرویس واقعی یا هوش مصنوعی.</p>
      <button disabled={busy} onClick={() => send("message")}>دریافت پیام نمونه</button>
      <button disabled={busy || !last.current} onClick={() => send("message", true)}>تکرار آخرین پیام</button>
      <button disabled={busy} onClick={() => send("failure")}>نمونهٔ خطای پردازش</button>
      <button disabled={busy} onClick={() => send("old")}>نمونهٔ پیش از اتصال</button>
      <Link href="/dashboard/connection">تنظیمات اتصال پیج</Link>
    </div>}
    {notice && <p role="status" className="shop-success">{notice}</p>}
    {submitError && <p role="alert" className="shop-error">{submitError}</p>}
    {error && <div role="alert" className="shop-error">{error}<button onClick={() => { void refresh().catch(() => setError("وضعیت پیام‌ها دریافت نشد. دوباره تلاش کنید.")); }}>تلاش دوباره</button></div>}
    {!data ? !error && <p role="status">در حال دریافت وضعیت پیام‌ها…</p> : data.items.length === 0 ? <p className="empty-work">هنوز پیامی دریافت نشده است. پس از اتصال پیج، یک پیام نمونه دریافت کنید.</p> : <ol className="message-list">{data.items.map(item => <li key={item.id}>
      <div><time dateTime={item.receivedAt}>{new Date(item.receivedAt).toLocaleString("fa-IR")}</time><span>{item.speaker === "shop" ? "فروشنده" : "مشتری"}</span></div>
      <strong className={`message-status ${item.status}`}>{statuses[item.status] || "وضعیت نامعلوم"}</strong>
      <p>{reasons[item.reason] || "وضعیت را دوباره بررسی کنید."}</p><small>تعداد تلاش: {item.attempts.toLocaleString("fa-IR")}</small>
    </li>)}</ol>}
  </section>;
}
