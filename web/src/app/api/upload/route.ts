import { env } from "@/lib/env";
import { buildForwardHeaders } from "@/lib/http/forward-headers";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * POST /api/upload — proxy a multipart file upload to the Go backend.
 *
 * Forwards the FormData to POST {goBackendUrl}/v1/attachments and returns the
 * JSON response. The upload target is one of: conversationId, or uploadBucket
 * (a pending pre-conversation upload). All FormData fields are forwarded
 * verbatim. Response shape: { id, url, mediaType, filename, size }.
 *
 * The returned `url` is rewritten to `/api/upload/<id>` so the browser always
 * fetches from the same origin.
 */
export async function POST(req: Request) {
  const formData = await req.formData();

  const goRes = await fetch(`${env.goBackendUrl}/v1/attachments`, {
    method: "POST",
    headers: buildForwardHeaders(req),
    body: formData,
  });

  if (!goRes.ok) {
    const text = await goRes.text().catch(() => "unknown error");
    return new Response(JSON.stringify({ error: text }), {
      status: goRes.status,
      headers: { "Content-Type": "application/json" },
    });
  }

  const data = (await goRes.json()) as {
    id: string;
    url: string;
    mediaType: string;
    filename: string;
    size: number;
  };

  // Rewrite the Go-issued URL to a same-origin BFF path.
  return Response.json({
    ...data,
    url: `/api/upload/${data.id}`,
  });
}
