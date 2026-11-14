import { authProxy } from "../../../../../../lib/auth";
async function proxy(request: Request, { params }: { params: Promise<{ store: string; path?: string[] }> }) {
  const { store, path = [] } = await params;
  if (path.length > 2) return Response.json({ code: "not_found" }, { status: 404 });
  return authProxy(request, `/stores/${encodeURIComponent(store)}/products${path.map(p => `/${encodeURIComponent(p)}`).join("")}${new URL(request.url).search}`);
}
export { proxy as GET, proxy as POST };
