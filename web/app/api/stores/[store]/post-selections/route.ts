import { authProxy } from "../../../../../lib/auth";
async function proxy(request: Request, { params }: { params: Promise<{ store: string }> }) {
  const { store } = await params;
  return authProxy(request, `/stores/${encodeURIComponent(store)}/post-selections${new URL(request.url).search}`);
}
export { proxy as GET, proxy as POST };
