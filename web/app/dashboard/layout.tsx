import { cookies, headers } from "next/headers";
import { safeNext } from "../../lib/auth";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";
export const dynamic = "force-dynamic";
export default async function PrivateLayout({ children }: { children: ReactNode }) {
  let status = 503;
  try {
    const response = await fetch(`${process.env.API_URL}/me`, { headers: { Cookie: (await cookies()).toString() }, cache: "no-store", signal: AbortSignal.timeout(5000) });
    status = response.status;
  } catch { /* Show a recoverable outage, not an authentication redirect loop. */ }
  if (status === 401) redirect(`/login?next=${encodeURIComponent(safeNext((await headers()).get("x-private-path")))}`);
  if (status !== 200) return <main><h1>ارتباط با فروشگاه برقرار نیست</h1><p>کمی بعد صفحه را دوباره بارگذاری کنید.</p></main>;
  return children;
}
