import { proxy } from "@/lib/api/bff";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Catch-all proxy for conversation history. Maps /api/conversations/* onto the Go
// backend's /v1/conversations/* endpoints (scoped to the authenticated user):
//   GET /api/conversations              -> GET /v1/conversations        (list)
//   GET /api/conversations/{id}/messages -> GET /v1/conversations/{id}/messages
function target(path?: string[]): string {
  const suffix =
    path && path.length ? "/" + path.map(encodeURIComponent).join("/") : "";
  return `/v1/conversations${suffix}`;
}

type Ctx = { params: Promise<{ path?: string[] }> };

export async function GET(req: Request, { params }: Ctx) {
  return proxy(req, "GET", target((await params).path));
}
