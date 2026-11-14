import { authProxy } from "../../../../lib/auth";
export const dynamic = "force-dynamic";
export async function POST(request: Request, context: { params: Promise<{ action: string }> }) {
  const { action } = await context.params;
  if (action !== "login" && action !== "logout") return new Response(null, { status: 404 });
  return authProxy(request, `/auth/${action}`);
}
