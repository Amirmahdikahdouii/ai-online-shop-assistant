"use client";

import { useCallback, useEffect, useState } from "react";
import { type Health, type State, unavailableHealth } from "./health";

const labels: Record<State, string> = {
  ready: "آماده",
  preparing: "در حال آماده‌سازی",
  error: "ارتباط برقرار نیست",
  unknown: "قابل بررسی نیست",
};

export default function Home() {
  const [health, setHealth] = useState<Health>();
  const [busy, setBusy] = useState(true);
  const [checkedAt, setCheckedAt] = useState<string>();
  const check = useCallback(async () => {
    setBusy(true);
    try {
      const response = await fetch("/api/health", {
        cache: "no-store",
        signal: AbortSignal.timeout(6000),
      });
      setHealth(await response.json());
      setCheckedAt(new Date().toLocaleTimeString("fa-IR"));
    } catch {
      setHealth(unavailableHealth);
    } finally {
      setBusy(false);
    }
  }, []);
  useEffect(() => {
    void check();
  }, [check]);
  const status = busy ? "preparing" : (health?.status ?? "unknown");

  return (
    <main>
      <header>
        <span className="brand">دستیار فروشگاه</span>
        <span className="mode">
          {health?.mode === "demo"
            ? "محیط نمایشی"
            : health?.mode === "live"
              ? "محیط زنده"
              : "محیط در حال بررسی"}
        </span>
      </header>
      <section className="intro">
        <h1>
          پیش از شروع،
          <br />
          ارتباط‌ها را بررسی کنیم.
        </h1>
        <p>
          آمادگی برنامه از روی ارتباط واقعی با پایگاه داده و پردازشگر بررسی
          می‌شود.
        </p>
      </section>
      <section
        className="health"
        aria-labelledby="health-title"
        aria-busy={busy}
      >
        <div className={`summary ${status}`} role="status" aria-live="polite">
          <span className="signal" aria-hidden="true" />
          <div>
            <h2 id="health-title">
              {busy
                ? "در حال بررسی ارتباط‌ها…"
                : status === "ready"
                  ? "برنامه آماده است"
                  : status === "preparing"
                    ? "برنامه در حال آماده‌سازی است"
                    : "برنامه هنوز آماده نیست"}
            </h2>
            <p>
              {busy
                ? "نتیجهٔ تازه تا چند لحظه دیگر نمایش داده می‌شود."
                : status === "ready"
                  ? "خواندن و نوشتن آزمایشی انجام شد؛ پردازشگر نیز فعال است."
                  : "سرویس‌ها را بررسی کنید و دوباره تلاش کنید. داده‌های قبلی حذف نمی‌شوند."}
            </p>
          </div>
        </div>
        <dl>
          {(
            [
              ["api", "رابط برنامه", "دسترسی وب به API"],
              [
                "database",
                "پایگاه داده",
                "ساختار معتبر و نوشتن / خواندن آزمایشی",
              ],
              ["worker", "پردازشگر", "آخرین نشانهٔ فعالیت در پایگاه داده"],
            ] as const
          ).map(([key, title, description]) => (
            <div className="row" key={key}>
              <div>
                <dt>{title}</dt>
                <dd>{description}</dd>
              </div>
              <span
                className={`badge ${busy ? "preparing" : (health?.[key] ?? "unknown")}`}
              >
                {busy ? "در حال بررسی" : labels[health?.[key] ?? "unknown"]}
              </span>
            </div>
          ))}
        </dl>
        <footer>
          <button onClick={check} disabled={busy}>
            {busy ? "در حال بررسی…" : "بررسی دوباره"}
          </button>
          <span>
            {checkedAt
              ? `آخرین بررسی: ${checkedAt}`
              : "هنوز نتیجه‌ای ثبت نشده است."}
          </span>
        </footer>
      </section>
      <aside>
        <h2>این بررسی چه چیزی را تغییر می‌دهد؟</h2>
        <p>
          فقط یک رکورد آزمایشی جدا از اطلاعات فروشگاه نوشته و خوانده می‌شود.
          هنوز محصول، مشتری یا سفارشی در این جریان ساخته نمی‌شود.
        </p>
        <p>اتصال سرویس‌های بیرونی در این مرحله بررسی نمی‌شود.</p>
      </aside>
    </main>
  );
}
