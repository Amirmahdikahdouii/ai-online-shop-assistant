import { authProxy } from "../../../../../lib/auth";
export async function GET(request: Request, { params }: { params: Promise<{ store: string }> }) {
  const { store } = await params;
  return authProxy(request, `/stores/${encodeURIComponent(store)}/processing`);
}
