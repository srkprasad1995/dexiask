import { proxy } from "@/lib/api/bff";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Catch-all proxy for the admin surface. Maps /api/admin/* onto the Go backend's
// top-level admin routes (which enforce the admin role):
//   GET/POST /api/admin/invites        -> /v1/invites
//   DELETE   /api/admin/invites/{login} -> /v1/invites/{login}
//   GET      /api/admin/users          -> /v1/users
function target(path?: string[]): string {
  const suffix =
    path && path.length ? "/" + path.map(encodeURIComponent).join("/") : "";
  return `/v1${suffix}`;
}

type Ctx = { params: Promise<{ path?: string[] }> };

export async function GET(req: Request, { params }: Ctx) {
  return proxy(req, "GET", target((await params).path));
}
export async function POST(req: Request, { params }: Ctx) {
  return proxy(req, "POST", target((await params).path));
}
export async function DELETE(req: Request, { params }: Ctx) {
  return proxy(req, "DELETE", target((await params).path));
}
