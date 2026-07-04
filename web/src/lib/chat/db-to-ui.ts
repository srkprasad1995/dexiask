import type { DexiaskUIMessage } from "@/types/chat";

export interface DbAttachment {
  id: string;
  filename: string;
  media_type: string;
  size: number;
}

/**
 * Shape of a message row returned by GET /v1/conversations/{id}/messages.
 */
export interface DbMessage {
  id: string;
  conversation_id: string;
  role: "user" | "assistant";
  content: string;
  seq: number;
  status: "running" | "complete" | "partial" | "error";
  model: string;
  created_at: string;
  attachments?: DbAttachment[];
}

/**
 * Map a persisted `DbMessage` to the AI SDK v6 `DexiaskUIMessage` format.
 *
 * Skips entirely empty messages (no content and no attachments).
 * A `status=running` row WITH content (partially generated) is rendered as-is
 * so the resume UI shows what was produced before the interruption.
 */
export function dbMessageToUI(msg: DbMessage): DexiaskUIMessage | null {
  const hasContent = msg.content.trim() !== "";
  const hasAttachments = (msg.attachments?.length ?? 0) > 0;

  // Skip entirely empty messages (no content, no attachments).
  if (!hasContent && !hasAttachments) return null;

  const parts: DexiaskUIMessage["parts"] = [];

  // File parts come first (shown above text in the user bubble).
  for (const att of msg.attachments ?? []) {
    parts.push({
      type: "file",
      url: `/api/upload/${att.id}`,
      mediaType: att.media_type,
      filename: att.filename,
    });
  }

  if (hasContent) {
    parts.push({ type: "text", text: msg.content });
  }

  return {
    id: msg.id,
    role: msg.role,
    parts,
    metadata: {
      model: msg.model || undefined,
      createdAt: msg.created_at
        ? new Date(msg.created_at).getTime()
        : undefined,
    },
  };
}

/**
 * Convert an ordered array of DB messages into UI messages.
 * Filters out nulls (empty / unrenderable rows).
 */
export function dbMessagesToUI(msgs: DbMessage[]): DexiaskUIMessage[] {
  return msgs
    .sort((a, b) => a.seq - b.seq) // belt-and-suspenders; Go already returns seq ASC
    .map(dbMessageToUI)
    .filter((m): m is DexiaskUIMessage => m !== null);
}
