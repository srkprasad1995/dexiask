import { proxy } from "@/lib/api/bff";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Catch-all proxy for the memory subtree. Requests go through the Go backend,
// which reverse-proxies to the memory service (/v1/memory/...) after injecting
// the authenticated user id. The browser never reaches the memory service.
//   GET  /api/memory/scopes        -> /v1/memory/scopes
//   GET  /api/memory/entries       -> /v1/memory/entries
//   GET  /api/memory/working       -> /v1/memory/working
//   POST /api/memory/consolidate   -> /v1/memory/consolidate
function target(path?: string[]): string {
  const suffix =
    path && path.length ? "/" + path.map(encodeURIComponent).join("/") : "";
  return `/v1/memory${suffix}`;
}

type Ctx = { params: Promise<{ path?: string[] }> };

export async function GET(req: Request, { params }: Ctx) {
  return proxy(req, "GET", target((await params).path));
}
export async function POST(req: Request, { params }: Ctx) {
  return proxy(req, "POST", target((await params).path));
}
export async function PUT(req: Request, { params }: Ctx) {
  return proxy(req, "PUT", target((await params).path));
}
export async function DELETE(req: Request, { params }: Ctx) {
  return proxy(req, "DELETE", target((await params).path));
}
