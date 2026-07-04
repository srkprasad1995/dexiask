import { proxy } from "@/lib/api/bff";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Catch-all proxy for the indexer subtree. Requests go through the Go backend,
// which reverse-proxies to the indexer service; the browser never reaches the
// indexer directly.
//   /api/indexer/repos    -> /v1/indexer/repos
//   /api/indexer/status   -> /v1/indexer/status
//   /api/indexer/reindex  -> /v1/indexer/reindex
//   /api/indexer/search   -> /v1/indexer/search
function target(path?: string[]): string {
  const suffix =
    path && path.length ? "/" + path.map(encodeURIComponent).join("/") : "";
  return `/v1/indexer${suffix}`;
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
