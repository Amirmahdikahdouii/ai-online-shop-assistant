import { authProxy } from "../../../../../../lib/auth";

async function proxy(request: Request, { params }: { params: Promise<{ store: string; action: string }> }) {
  const { store, action } = await params;
  return authProxy(request, `/stores/${encodeURIComponent(store)}/connection/${encodeURIComponent(action)}`);
}
export const GET = proxy;
export const POST = proxy;
