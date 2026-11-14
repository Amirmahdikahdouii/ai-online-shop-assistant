import { NextRequest, NextResponse } from "next/server";
export function proxy(request: NextRequest) {
  // Preserve the requested path; the server layout performs authoritative
  // database validation even when an attacker supplies an arbitrary cookie.
  if (!request.cookies.get("shop_session")) {
    const url = new URL("/login", request.url);
    url.searchParams.set("next", request.nextUrl.pathname + request.nextUrl.search);
    return NextResponse.redirect(url);
  }
  const headers = new Headers(request.headers);
  headers.set("x-private-path", request.nextUrl.pathname + request.nextUrl.search);
  return NextResponse.next({ request: { headers } });
}
export const config = { matcher: ["/dashboard/:path*"] };
