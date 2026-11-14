export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const requestId = crypto.randomUUID();
  const fail = (status: number, code: string) => Response.json(
    { code, fieldErrors: null, requestId },
    { status, headers: { "Cache-Control": "no-store" } },
  );
  // Browser mutations must originate here. Non-browser API clients may omit Origin.
  const origin = request.headers.get("origin");
  const expectedOrigin = `${new URL(request.url).protocol}//${request.headers.get("host")}`;
  if ((origin && origin !== expectedOrigin) ||
      request.headers.get("sec-fetch-site") === "cross-site") return fail(403, "origin_rejected");
  if (request.headers.get("content-type")?.split(";")[0].trim() !== "application/json") return fail(415, "json_required");
  const reader = request.body?.getReader();
  if (!reader) return fail(400, "invalid_json");
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > 4096) { await reader.cancel(); return fail(413, "body_too_large"); }
      chunks.push(value);
    }
    const response = await fetch(`${process.env.API_URL}/auth/register`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: Buffer.concat(chunks), cache: "no-store", signal: AbortSignal.timeout(10000),
    });
    return Response.json(await response.json(), {
      status: response.status,
      headers: { "Cache-Control": "no-store", ...(response.headers.has("retry-after") ? { "Retry-After": response.headers.get("retry-after")! } : {}) },
    });
  } catch { return fail(503, "registration_unavailable"); }
}
