/**
 * Conversation history hooks, via the same-origin BFF proxy under
 * /api/conversations/*, which the Go backend scopes to the authenticated user.
 */

"use client";

import { useQuery } from "@tanstack/react-query";

import { apiGet } from "@/lib/api/fetcher";
import { dbMessagesToUI, type DbMessage } from "@/lib/chat/db-to-ui";
import type { DexiaskUIMessage } from "@/types/chat";

export interface Conversation {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

export const conversationKeys = {
  list: () => ["conversations", "list"] as const,
};

/** The user's conversations, most-recently-updated first. */
export function useConversations() {
  return useQuery({
    queryKey: conversationKeys.list(),
    queryFn: () =>
      apiGet<{ conversations: Conversation[] }>(
        "/api/conversations?page_size=100",
      ).then((r) => r.conversations ?? []),
    staleTime: 5_000,
    retry: false,
  });
}

/** Fetch a conversation's transcript as UI messages, ready for useChat. */
export async function fetchConversationMessages(
  id: string,
): Promise<DexiaskUIMessage[]> {
  const { messages } = await apiGet<{ messages: DbMessage[] }>(
    `/api/conversations/${encodeURIComponent(id)}/messages`,
  );
  return dbMessagesToUI(messages ?? []);
}
