import { authProxy } from "../../../../../../../lib/auth";
async function proxy(request: Request, { params }: { params: Promise<{ store: string; selection: string }> }) {
 const { store, selection } = await params;
 return authProxy(request, `/stores/${encodeURIComponent(store)}/post-selections/${encodeURIComponent(selection)}/mapping`);
}
export const GET = proxy;
export const POST = proxy;
