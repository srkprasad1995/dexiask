import { createUIMessageStream, createUIMessageStreamResponse } from "ai";

import type { DexiaskUIMessage } from "@/types/chat";
import { env } from "@/lib/env";
import { buildForwardHeaders } from "@/lib/http/forward-headers";
import { pumpGoStream } from "@/lib/chat/go-sse-to-ui";
import { logger } from "@/lib/observability/logger";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * POST /api/chat — start or continue a conversation turn.
 *
 * The browser sends only the **new user message** + an optional conversationId.
 * The Go backend owns history — it persists every turn and resumes the engine
 * session natively.
 *
 * NOTE: We deliberately do NOT forward `req.signal` to Go. The abort signal
 * from the browser means "I disconnected" (not "stop the run"). Go generation
 * is decoupled from the HTTP connection; to explicitly cancel a run use the
 * POST /api/chat/stop endpoint.
 */
export async function POST(req: Request) {
  const rawBody = await req.json();

  // Shape of the v6 UI-message parts/messages the transport posts. Untyped JSON
  // off the wire, so we describe the fields we read with all keys optional.
  type RawPart = {
    type?: string;
    text?: string;
    url?: string;
    mediaType?: string;
    filename?: string;
  };
  type RawMessage = { role?: string; content?: string; parts?: RawPart[] };

  // The transport sends only the last user turn (via prepareSendMessagesRequest).
  // Flatten v6 parts → content string AND extract file attachment refs.
  const rawMessages: RawMessage[] = rawBody.messages ?? [];
  const messages = rawMessages.map((m) => {
    const parts = m.parts ?? [];
    const content =
      m.content ||
      parts
        .filter((p) => p.type === "text")
        .map((p) => p.text ?? "")
        .join("") ||
      "";
    const attachments = parts
      .filter((p) => p.type === "file")
      .map((p) => ({
        // Extract the attachment ID from the BFF URL /api/upload/<id>
        id: (p.url ?? "").split("/").pop() ?? "",
        mediaType: p.mediaType ?? "",
        filename: p.filename ?? "",
        url: p.url ?? "",
      }));
    return { role: m.role ?? "", content, attachments };
  });

  const body = {
    conversationId: rawBody.conversationId ?? null,
    uploadBucket: rawBody.uploadBucket ?? null,
    messages,
  };

  // No signal: disconnect ≠ stop. Use POST /api/chat/stop to cancel a run.
  const goRes = await fetch(`${env.goBackendUrl}/v1/chat/stream`, {
    method: "POST",
    headers: {
      ...buildForwardHeaders(req),
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify(body),
  });

  if (!goRes.ok || !goRes.body) {
    logger.error("chat start: backend error", {
      status: goRes.status,
      conversationId: body.conversationId,
    });
    return new Response(
      JSON.stringify({ error: `Backend error (${goRes.status})` }),
      { status: 502, headers: { "Content-Type": "application/json" } },
    );
  }

  const stream = createUIMessageStream<DexiaskUIMessage>({
    execute: async ({ writer }) => {
      writer.write({ type: "start" });
      await pumpGoStream(goRes.body!, writer);
      writer.write({ type: "finish" });
    },
    onError: (err) => (err instanceof Error ? err.message : "stream error"),
  });

  return createUIMessageStreamResponse({ stream });
}

/**
 * GET /api/chat?conversationId=<id> — reconnect to an in-progress run.
 *
 * Called automatically by `useChat({ resume: true })` on mount when a
 * conversationId is known. Go's run manager replays buffered events from the
 * beginning of the run, then continues streaming live events. Returns 204 if
 * no active run exists.
 */
export async function GET(req: Request) {
  const { searchParams } = new URL(req.url);
  const conversationId = searchParams.get("conversationId");

  if (!conversationId) {
    // No conversation to resume — treat as no active run.
    return new Response(null, { status: 204 });
  }

  const goUrl = new URL(`${env.goBackendUrl}/v1/chat/stream`);
  goUrl.searchParams.set("conversationId", conversationId);

  // Forward Last-Event-ID so Go can replay buffered events from the cursor.
  const lastEventId = req.headers.get("last-event-id") ?? "";
  const goRes = await fetch(goUrl.toString(), {
    method: "GET",
    headers: {
      ...buildForwardHeaders(req),
      Accept: "text/event-stream",
      ...(lastEventId ? { "Last-Event-ID": lastEventId } : {}),
    },
  });

  // No active run, or conversation not found yet (fresh id from client redirect).
  if (goRes.status === 204 || goRes.status === 404) {
    return new Response(null, { status: 204 });
  }

  if (!goRes.ok || !goRes.body) {
    logger.error("chat resume: backend error", {
      status: goRes.status,
      conversationId,
    });
    return new Response(
      JSON.stringify({ error: `Backend error (${goRes.status})` }),
      { status: 502, headers: { "Content-Type": "application/json" } },
    );
  }

  const stream = createUIMessageStream<DexiaskUIMessage>({
    execute: async ({ writer }) => {
      writer.write({ type: "start" });
      await pumpGoStream(goRes.body!, writer);
      writer.write({ type: "finish" });
    },
    onError: (err) => (err instanceof Error ? err.message : "stream error"),
  });

  return createUIMessageStreamResponse({ stream });
}
