"use client";
import { useState } from "react";
export default function Dashboard() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function logout() {
    if (busy) return; setBusy(true); setError("");
    try {
      const me = await fetch("/api/me", { cache: "no-store" });
      if (me.status === 401) { window.location.replace("/login"); return; }
      if (!me.ok) throw new Error();
      const { csrfToken } = await me.json();
      const response = await fetch("/api/auth/logout", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken }, body: "{}" });
      if (response.ok || response.status === 401) { window.location.replace("/login"); return; }
      throw new Error();
    } catch { setError("خروج انجام نشد؛ دوباره تلاش کنید."); setBusy(false); }
  }
  return <main><header><span className="brand">دستیار فروشگاه</span><button onClick={logout} disabled={busy}>{busy ? "در حال خروج…" : "خروج"}</button></header><h1>فروشگاه‌های شما</h1><p>هنوز فروشگاهی به حساب شما متصل نشده است.</p>{error && <p role="alert">{error}</p>}</main>;
}
