import { env } from "@/lib/env";
import { buildForwardHeaders } from "@/lib/http/forward-headers";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * GET /api/upload/[id] — proxy attachment file serve from the Go backend.
 *
 * Streams GET {goBackendUrl}/v1/attachments/{id} back to the browser,
 * preserving Content-Type and Content-Disposition headers.
 */
export async function GET(
  req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;

  const goRes = await fetch(`${env.goBackendUrl}/v1/attachments/${id}`, {
    method: "GET",
    headers: buildForwardHeaders(req),
  });

  if (!goRes.ok) {
    return new Response(null, { status: goRes.status });
  }

  const contentType =
    goRes.headers.get("Content-Type") ?? "application/octet-stream";
  const disposition = goRes.headers.get("Content-Disposition") ?? "";

  const headers = new Headers({
    "Content-Type": contentType,
    "Cache-Control": "private, max-age=3600",
  });
  if (disposition) {
    headers.set("Content-Disposition", disposition);
  }

  return new Response(goRes.body, { headers });
}
