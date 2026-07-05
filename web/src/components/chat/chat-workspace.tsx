"use client";

import { useCallback, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { conversationKeys } from "@/lib/api/conversations";
import { Chat } from "@/components/chat/chat";
import { ConversationSidebar } from "@/components/chat/conversation-sidebar";

/**
 * Chat with a history rail. Owns which conversation is open:
 *   - `sessionKey` remounts <Chat> on an explicit open/new so its useChat state
 *     is fresh (and it never remounts mid-stream).
 *   - `loadId` is the conversation the freshly-mounted Chat loads (null = new).
 *   - `activeId` drives the sidebar highlight; it also updates (without a
 *     remount) when a brand-new chat is assigned an id, and the list refetches.
 */
export function ChatWorkspace({ suggestions = [] }: { suggestions?: string[] }) {
  const qc = useQueryClient();
  const [sessionKey, setSessionKey] = useState(0);
  const [loadId, setLoadId] = useState<string | null>(null);
  const [activeId, setActiveId] = useState<string | null>(null);

  const openConversation = useCallback((id: string) => {
    setLoadId(id);
    setActiveId(id);
    setSessionKey((k) => k + 1);
  }, []);

  const newChat = useCallback(() => {
    setLoadId(null);
    setActiveId(null);
    setSessionKey((k) => k + 1);
  }, []);

  const onConversationCreated = useCallback(
    (id: string) => {
      setActiveId(id);
      qc.invalidateQueries({ queryKey: conversationKeys.list() });
    },
    [qc],
  );

  return (
    <div className="flex h-full min-h-0">
      <aside className="hidden w-64 shrink-0 border-r bg-sidebar md:block">
        <ConversationSidebar
          selectedId={activeId}
          onSelect={openConversation}
          onNew={newChat}
        />
      </aside>
      <div className="min-w-0 flex-1">
        <Chat
          key={sessionKey}
          suggestions={suggestions}
          loadConversationId={loadId}
          onConversationCreated={onConversationCreated}
        />
      </div>
    </div>
  );
}
