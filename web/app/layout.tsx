import type { Metadata } from "next";
import "@fontsource/vazirmatn/400.css";
import "@fontsource/vazirmatn/600.css";
import "./style.css";

export const metadata: Metadata = {
  title: "سلامت دستیار فروشگاه",
  description: "بررسی آمادگی برنامه و ارتباط پایگاه داده",
};
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="fa" dir="rtl">
      <body>{children}</body>
    </html>
  );
}
