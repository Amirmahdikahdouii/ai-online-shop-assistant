export function safeNext(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//") || /[\\\u0000-\u0020]/.test(value)) return "/dashboard";
  try {
    const url = new URL(value, "http://internal");
    if (/%(?:2f|5c|2e)/i.test(url.pathname)) return "/dashboard";
    const path = decodeURIComponent(url.pathname);
    if (url.origin !== "http://internal" || !path.startsWith("/dashboard") || (path !== "/dashboard" && !path.startsWith("/dashboard/"))) return "/dashboard";
    return url.pathname + url.search;
  } catch { return "/dashboard"; }
}

export async function authProxy(request: Request, path: string) {
  const fail = (status: number, code: string) => Response.json({ code, fieldErrors: null, requestId: crypto.randomUUID() }, { status, headers: { "Cache-Control": "no-store" } });
  const origin = process.env.APP_ORIGIN || "http://localhost:3000";
  const bodyLimit = /^\/stores\/[^/]+\/products(?:\/|$)/.test(path) ? 2 * 1024 * 1024 : 4096;
  if (request.method === "POST" && (request.headers.get("origin") !== origin || request.headers.get("sec-fetch-site") === "cross-site")) return fail(403, "origin_rejected");
  try {
    let body: Buffer<ArrayBuffer> | undefined;
    if (request.method === "POST") {
      const reader = request.body?.getReader();
      const chunks: Uint8Array[] = []; let size = 0;
      if (reader) while (true) {
        const { value, done } = await reader.read(); if (done) break;
        size += value.length;
        if (size > bodyLimit) { await reader.cancel(); return fail(413, "body_too_large"); }
        chunks.push(value);
      }
      body = Buffer.concat(chunks);
    }
    const response = await fetch(`${process.env.API_URL}${path}`, {
      method: request.method, cache: "no-store", signal: AbortSignal.timeout(10000),
      headers: { "Content-Type": request.headers.get("content-type") || "application/json", Origin: origin,
        Cookie: request.headers.get("cookie") || "", "X-CSRF-Token": request.headers.get("x-csrf-token") || "" },
      body,
    });
    const headers = new Headers({ "Cache-Control": "no-store" });
    for (const name of ["set-cookie", "retry-after", "x-request-id"]) {
      const value = response.headers.get(name); if (value) headers.set(name, value);
    }
    return Response.json(await response.json(), { status: response.status, headers });
  } catch { return fail(503, "auth_unavailable"); }
}
