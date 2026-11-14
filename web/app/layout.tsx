import type { Metadata } from "next";
import "@fontsource/vazirmatn/400.css";
import "@fontsource/vazirmatn/600.css";
import "./style.css";

export const metadata: Metadata = {
  title: "دستیار فروشگاه",
  description: "دستیار مدیریت سفارش‌های فروشگاه اینستاگرام",
};
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="fa" dir="rtl">
      <body>{children}</body>
    </html>
  );
}
