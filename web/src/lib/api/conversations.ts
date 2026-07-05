/**
 * Conversation history hooks, via the same-origin BFF proxy under
 * /api/conversations/*, which the Go backend scopes to the authenticated user.
 */

"use client";

import { useInfiniteQuery } from "@tanstack/react-query";

import { apiGet } from "@/lib/api/fetcher";
import { dbMessagesToUI, type DbMessage } from "@/lib/chat/db-to-ui";
import type { DexiaskUIMessage } from "@/types/chat";

export interface Conversation {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

interface ConversationsPage {
  conversations: Conversation[];
  next_page_token: string;
}

const PAGE_SIZE = 25;

export const conversationKeys = {
  list: () => ["conversations", "list"] as const,
};

/**
 * The user's conversations, most-recently-updated first, paginated via the
 * backend's page_token cursor. Returns an infinite query — call fetchNextPage()
 * to load older conversations.
 */
export function useConversations() {
  return useInfiniteQuery({
    queryKey: conversationKeys.list(),
    queryFn: ({ pageParam }) => {
      const token = pageParam
        ? `&page_token=${encodeURIComponent(pageParam)}`
        : "";
      return apiGet<ConversationsPage>(
        `/api/conversations?page_size=${PAGE_SIZE}${token}`,
      );
    },
    initialPageParam: "",
    getNextPageParam: (last) => last.next_page_token || undefined,
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
