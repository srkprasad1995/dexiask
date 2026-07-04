import { env } from "@/lib/env";
import { buildForwardHeaders } from "@/lib/http/forward-headers";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * POST /api/chat/stop?conversationId=<id>
 *
 * Explicitly stops the in-progress generation for a conversation. This cancels
 * the run on the Go side. The persisted assistant message is marked partial.
 *
 * Note: this is distinct from the browser disconnecting (abort signal).
 * Disconnect = unsubscribe only; the Go run continues. Only this endpoint
 * actually cancels the backend run.
 */
export async function POST(req: Request) {
  const { searchParams } = new URL(req.url);
  const conversationId = searchParams.get("conversationId");

  if (!conversationId) {
    return new Response(
      JSON.stringify({ error: "conversationId is required" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  const goUrl = new URL(`${env.goBackendUrl}/v1/chat/stop`);
  goUrl.searchParams.set("conversationId", conversationId);

  const goRes = await fetch(goUrl.toString(), {
    method: "POST",
    headers: buildForwardHeaders(req),
  });

  if (!goRes.ok && goRes.status !== 404) {
    return new Response(
      JSON.stringify({ error: `Backend error (${goRes.status})` }),
      { status: goRes.status, headers: { "Content-Type": "application/json" } },
    );
  }

  // 404 from Go = no active run (already finished). Treat as success.
  return new Response(null, { status: 200 });
}
