"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";

type Connection = { id: string; pageId: string; accountId: string; status: string; receivingSince: string };
export default function ConnectionSettings({ store, csrf }: { store: string; csrf: string }) {
  const [data, setData] = useState<{ connection: Connection | null; fakeAvailable: boolean } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const locked = useRef(false);
  const base = `/api/stores/${encodeURIComponent(store)}/connection`;
  async function refresh() {
    const response = await fetch(`${base}/status`, { cache: "no-store" });
    if (response.status === 401) { window.location.replace("/login"); return; }
    if (!response.ok) throw new Error();
    setData(await response.json());
  }
  useEffect(() => { void refresh().catch(() => setError("وضعیت اتصال دریافت نشد؛ دوباره تلاش کنید.")); }, [store]); // eslint-disable-line react-hooks/exhaustive-deps
  async function change(disconnect: boolean) {
    if (locked.current) return;
    locked.current = true; setBusy(true); setError("");
    async function post(action: string, body: object) {
      const response = await fetch(`${base}/${action}`, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) });
      const result = await response.json();
      if (!response.ok) throw new Error(result.code);
      return result;
    }
    try {
      if (disconnect) await post("disconnect", {});
      else {
        const { state } = await post("start", {});
        await post("callback", { state, code: connection?.pageId ?? "demo-page" });
      }
      await refresh();
    } catch (e) {
      setError(e instanceof Error && e.message === "page_already_linked" ? "این پیج آزمایشی به فروشگاه دیگری متصل است." : "تغییر اتصال انجام نشد. وضعیت را بررسی و دوباره تلاش کنید.");
    } finally { locked.current = false; setBusy(false); }
  }
  const connection = data?.connection;
  return <section className="empty-work" aria-busy={busy}>
    <h2>اتصال پیج اینستاگرام</h2>
    {data?.fakeAvailable && <p className="connection-note">محیط شبیه‌سازی‌شده — هیچ ارتباطی با اینستاگرام واقعی برقرار نمی‌شود.</p>}
    {error && <p role="alert" className="shop-error">{error}</p>}
    {!data ? <><p role="status">در حال دریافت وضعیت اتصال…</p>{error && <button onClick={() => { setError(""); void refresh().catch(() => setError("وضعیت اتصال دریافت نشد؛ دوباره تلاش کنید.")); }}>تلاش دوباره</button>}</> : <>
      <p role="status">وضعیت فعلی: {connection ? connection.status === "connected" ? "متصل" : "قطع‌شده" : "بدون اتصال"}</p>
      {connection && <dl><dt>پیج آزمایشی</dt><dd dir="ltr">{connection.pageId}</dd><dt>شروع دریافت</dt><dd>{new Date(connection.receivingSince).toLocaleString("fa-IR")}</dd></dl>}
      <p>قطع اتصال، نشست و داده‌های فروشگاه را حذف نمی‌کند. اتصال مجدد به همین پیج، سابقه را حفظ می‌کند؛ تعویض پیج خودکار نیست.</p>
      {data.fakeAvailable ? <button className="shop-action" disabled={busy} onClick={() => change(connection?.status === "connected")}>{busy ? "در حال تغییر اتصال…" : connection?.status === "connected" ? "قطع اتصال آزمایشی" : connection ? "اتصال مجدد همین پیج" : "اتصال پیج آزمایشی"}</button> : <p>اتصال واقعی هنوز پیکربندی نشده است.</p>}
    </>}
    <p><Link href="/dashboard">بازگشت به سفارش‌ها</Link></p>
  </section>;
}
