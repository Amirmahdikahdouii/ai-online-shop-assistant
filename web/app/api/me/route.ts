import { authProxy } from "../../../lib/auth";
export const dynamic = "force-dynamic";
export function GET(request: Request) { return authProxy(request, "/me"); }
