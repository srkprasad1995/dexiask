import { proxy } from "@/lib/api/bff";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Catch-all proxy for the custom MCP servers subtree. Requests go through the
// Go backend, which owns the CRUD store; the browser never reaches it directly.
//   GET    /api/mcp        -> GET    /v1/mcp-servers
//   POST   /api/mcp        -> POST   /v1/mcp-servers
//   PUT    /api/mcp/{id}   -> PUT    /v1/mcp-servers/{id}
//   DELETE /api/mcp/{id}   -> DELETE /v1/mcp-servers/{id}
function target(path?: string[]): string {
  const suffix =
    path && path.length ? "/" + path.map(encodeURIComponent).join("/") : "";
  return `/v1/mcp-servers${suffix}`;
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
