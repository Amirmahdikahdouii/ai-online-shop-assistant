import { authProxy } from "../../../../../../../lib/auth";
export async function POST(request: Request, { params }: { params: Promise<{ store: string; selection: string }> }) {
 const { store, selection } = await params;
 return authProxy(request, `/stores/${encodeURIComponent(store)}/post-selections/${encodeURIComponent(selection)}/retry`);
}
