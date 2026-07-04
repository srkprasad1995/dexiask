import type { UIMessage } from "ai";

/**
 * Custom streamed "data" parts. These ride the AI SDK UI message stream as
 * `data-<name>` chunks. Dexiask keeps a single one: the backend-assigned
 * conversation id, emitted as the first frame of a turn and captured by
 * `chat.tsx` to make the URL bookmarkable/resumable.
 */
export type DexiaskDataParts = {
  /** Emitted as the first SSE frame when the backend creates/resolves a conversation. */
  conversation: { id: string };
};

/** Optional per-message metadata (model, timing). */
export interface DexiaskMessageMetadata {
  model?: string;
  createdAt?: number;
}

/** The concrete UIMessage type used across the chat UI. */
export type DexiaskUIMessage = UIMessage<
  DexiaskMessageMetadata,
  DexiaskDataParts
>;
