"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import "./register.css";

type Fields = { username?: string; password?: string };
export default function Register() {
  const [fields, setFields] = useState<Fields>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [visible, setVisible] = useState(false);
  const [user, setUser] = useState("");
  const inFlight = useRef(false);
  const username = useRef<HTMLInputElement>(null);
  const password = useRef<HTMLInputElement>(null);
  const confirmation = useRef<HTMLHeadingElement>(null);
  useEffect(() => { if (user) confirmation.current?.focus(); }, [user]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (inFlight.current || user) return;
    inFlight.current = true;
    setBusy(true); setFields({}); setError("");
    try {
      const response = await fetch("/api/auth/register", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.current!.value, password: password.current!.value }),
        signal: AbortSignal.timeout(15000),
      });
      const result = await response.json();
      if (response.status === 201 && result.user?.username) {
        password.current!.value = "";
        setUser(result.user.username);
      } else if ((response.status === 409 || response.status === 422) && result.fieldErrors) {
        setFields(result.fieldErrors);
        (result.fieldErrors.username ? username : password).current?.focus();
      } else {
        setError(response.status === 429 ? "درخواست‌های زیادی در حال انجام است؛ چند لحظه بعد دوباره تلاش کنید." : "ثبت‌نام تأیید نشد. دوباره تلاش کنید؛ اگر نام کاربری ثبت شده بود، حساب دیگری ساخته نمی‌شود.");
      }
    } catch {
      setError("ارتباط برقرار نشد و نتیجهٔ ثبت‌نام مشخص نیست. اتصال را بررسی و دوباره تلاش کنید؛ از همان نام کاربری استفاده کنید.");
    } finally { inFlight.current = false; setBusy(false); }
  }

  return <main className="registration">
    <header><span className="brand">دستیار فروشگاه</span><span className="mode">شروع با یک حساب شخصی</span></header>
    <div className="register-layout">
      <section className="register-form" aria-labelledby="register-title">
        {user ? <div className="registered" role="status">
          <h1 id="register-title" ref={confirmation} tabIndex={-1}>حساب شما ساخته شد</h1>
          <p>نام کاربری شما: <bdi className="registered-name">{user}</bdi></p>
          <p>نام کاربری و رمزتان را در جای امن نگه دارید. بازیابی رمز در این سرویس وجود ندارد.</p>
          <p>ثبت‌نام انجام شد. برای ادامه <a href="/login">وارد حساب خود شوید</a>.</p>
        </div> : <>
          <h1 id="register-title">حساب خود را بسازید</h1>
          <p className="register-lead">برای شروع، فقط یک نام کاربری و رمز عبور لازم دارید.</p>
          <form onSubmit={submit} aria-busy={busy} noValidate>
            <div className="register-field">
              <label htmlFor="username">نام کاربری</label>
              <input ref={username} id="username" name="username" dir="auto" autoComplete="username" autoCapitalize="none" spellCheck={false} maxLength={128} aria-invalid={!!fields.username} aria-describedby={`username-hint${fields.username ? " username-error" : ""}`} readOnly={busy} />
              <p id="username-hint">۳ تا ۳۲ نویسه؛ حروف فارسی یا لاتین، عدد، نقطه، خط تیره و زیرخط. بدون فاصله.</p>
              {fields.username && <p className="field-error" id="username-error">{fields.username}</p>}
            </div>
            <div className="register-field">
              <label htmlFor="password">رمز عبور</label>
            <input ref={password} id="password" name="password" type={visible ? "text" : "password"} dir="ltr" autoComplete="new-password" maxLength={256} aria-invalid={!!fields.password} aria-describedby={`password-hint${fields.password ? " password-error" : ""}`} readOnly={busy} />
            <button type="button" className="password-toggle" aria-pressed={visible} aria-controls="password" onClick={() => setVisible(!visible)}>{visible ? "پنهان کردن رمز" : "نمایش رمز"}</button>
              <p id="password-hint">۱۲ تا ۱۲۸ نویسه. رمزی انتخاب کنید که در جای دیگری استفاده نکرده‌اید.</p>
              {fields.password && <p className="field-error" id="password-error">{fields.password}</p>}
            </div>
            <p className="password-notice">ایمیل یا شماره تلفن نمی‌گیریم؛ امکان بازیابی رمز هم وجود ندارد. رمزتان را در جای امن نگه دارید.</p>
            {error && <p className="register-error" role="alert">{error}</p>}
            <div aria-live="polite">{busy && <p className="sending">در حال ساخت حساب؛ لطفاً منتظر بمانید…</p>}</div>
            <button type="submit" disabled={busy}>{busy ? "در حال ساخت حساب…" : "ساخت حساب"}</button>
          </form>
        </>}
      </section>
      <aside className="register-context">
        <h2>از پیام مشتری تا پیگیری سفارش</h2>
        <p>دستیار فروشگاه برای مرتب‌کردن اطلاعات سفارش‌های اینستاگرام است. تصمیم‌های مهم، مثل تأیید پرداخت و ثبت ارسال، با شما می‌ماند.</p>
        <div className="username-example"><h3>چه نامی انتخاب کنم؟</h3><p>نامی که راحت به خاطر می‌سپارید؛ مثلاً برای مدیر فروشگاه نارون:</p><bdi dir="ltr">narvan.shop</bdi><p>این فقط یک مثال است. نام کاربری شما مستقل از نام پیج اینستاگرام است.</p></div>
      </aside>
    </div>
  </main>;
}
