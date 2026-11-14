import { authProxy } from "../../../lib/auth";
export async function POST(request: Request) { return authProxy(request, "/stores"); }
