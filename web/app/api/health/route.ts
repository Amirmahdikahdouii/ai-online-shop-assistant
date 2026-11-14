import { unavailableHealth } from "../../health";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const response = await fetch(`${process.env.API_URL}/readyz`, {
      cache: "no-store",
      signal: AbortSignal.timeout(4500),
    });
    const health = await response.json();
    return Response.json(health, {
      status: response.status,
      headers: { "Cache-Control": "no-store" },
    });
  } catch {
    return Response.json(unavailableHealth, {
      status: 503,
      headers: { "Cache-Control": "no-store" },
    });
  }
}
