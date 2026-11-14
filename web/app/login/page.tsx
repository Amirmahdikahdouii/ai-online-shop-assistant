"use client";
import { useRef, useState, type FormEvent } from "react";
import { safeNext } from "../../lib/auth";
import "../register/register.css";

export default function Login() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [visible, setVisible] = useState(false);
  const pending = useRef(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (pending.current) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    pending.current = true; setBusy(true); setError("");
    try {
      const response = await fetch("/api/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username: data.get("username"), password: data.get("password") }), signal: AbortSignal.timeout(15000) });
      if (response.ok) { form.reset(); window.location.assign(safeNext(new URLSearchParams(window.location.search).get("next"))); return; }
      setError(response.status === 401 ? "نام کاربری یا رمز عبور درست نیست." : response.status === 429 ? "تلاش‌های زیادی انجام شده است؛ کمی بعد دوباره امتحان کنید." : "ورود ممکن نشد؛ دوباره تلاش کنید.");
    } catch { setError("ارتباط برقرار نشد؛ اتصال را بررسی و دوباره تلاش کنید."); }
    pending.current = false; setBusy(false);
    form.querySelector<HTMLInputElement>("#password")?.focus();
  }
  return <main className="registration"><header><a className="brand" href="/">دستیار فروشگاه</a><a href="/register">ساخت حساب</a></header>
    <div className="register-layout"><section className="register-form"><h1>ورود به فروشگاه</h1><p className="register-lead">با نام کاربری و رمز عبور خود وارد شوید.</p>
      <form onSubmit={submit} aria-busy={busy}>
        <div className="register-field"><label htmlFor="username">نام کاربری</label><input id="username" name="username" required autoComplete="username" autoCapitalize="none" spellCheck={false} dir="auto" readOnly={busy} /></div>
        <div className="register-field"><label htmlFor="password">رمز عبور</label><input id="password" name="password" required type={visible ? "text" : "password"} autoComplete="current-password" dir="ltr" readOnly={busy} aria-invalid={!!error} aria-describedby={error ? "login-error" : undefined} /><button type="button" className="password-toggle" aria-pressed={visible} aria-controls="password" onClick={() => setVisible(!visible)}>{visible ? "پنهان کردن رمز" : "نمایش رمز"}</button>
        {error && <p id="login-error" role="alert" className="field-error">{error}</p>}</div>
        <button disabled={busy} type="submit">{busy ? "در حال ورود…" : "ورود"}</button>
      </form></section><aside className="register-context"><h2>مدیریت فروشگاه، در دسترس شما</h2><p>قطع اتصال پیج، شما را از حساب خارج نمی‌کند.</p><p className="password-notice">بازیابی رمز عبور در این نسخه وجود ندارد. رمز خود را در جای امن نگه دارید.</p></aside></div></main>;
}
