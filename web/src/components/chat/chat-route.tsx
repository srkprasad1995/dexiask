"use client";

import { useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";

import { conversationKeys } from "@/lib/api/conversations";
import { Chat } from "@/components/chat/chat";

/**
 * Chat page content. The open conversation is driven by the `?c=<id>` URL param
 * (set by the sidebar history), so opening a conversation works from any page.
 * When a brand-new chat is assigned an id, the URL is updated in place (no
 * remount) and the sidebar list refetches.
 */
export function ChatRoute({ suggestions = [] }: { suggestions?: string[] }) {
  const router = useRouter();
  const qc = useQueryClient();
  const conversationId = useSearchParams().get("c");

  const onConversationCreated = useCallback(
    (id: string) => {
      router.replace(`/?c=${encodeURIComponent(id)}`, { scroll: false });
      qc.invalidateQueries({ queryKey: conversationKeys.list() });
    },
    [router, qc],
  );

  return (
    <Chat
      suggestions={suggestions}
      loadConversationId={conversationId}
      onConversationCreated={onConversationCreated}
    />
  );
}
