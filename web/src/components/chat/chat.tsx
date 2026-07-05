"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useChat } from "@ai-sdk/react";
import { DefaultChatTransport } from "ai";
import type { FileUIPart } from "ai";
import { RefreshCw } from "lucide-react";
import { toast } from "sonner";

import type { DexiaskUIMessage } from "@/types/chat";
import { fetchConversationMessages } from "@/lib/api/conversations";
import { MessageList } from "@/components/chat/message-list";
import { ChatEmptyState } from "@/components/chat/chat-empty-state";
import { Composer } from "@/components/chat/composer";
import { Button } from "@/components/ui/button";

/**
 * Chat — the core conversation UI.
 *
 * Design:
 * - The browser is STATELESS: only the new user message is sent per turn.
 *   The backend reconstructs context from the persisted transcript.
 * - `conversationId` is server-issued (first SSE frame `data-conversation`).
 *   We store it in a ref and include it in every outgoing request so the
 *   backend resolves the right conversation without the full history in the body.
 * - The stop button calls the explicit `/api/chat/stop` endpoint (not just
 *   `useChat.stop()`) so the backend run is actually cancelled.
 */
export function Chat({
  suggestions = [],
  loadConversationId = null,
  onConversationCreated,
}: {
  suggestions?: string[];
  /** When set, load this conversation's transcript on mount (history reopen). */
  loadConversationId?: string | null;
  /** Called when a brand-new conversation is assigned an id by the backend. */
  onConversationCreated?: (id: string) => void;
}) {
  // Tracks the backend-assigned conversation ID across renders. Seeded null on
  // a new chat; captured from the first `data-conversation` SSE frame so
  // follow-up turns attach to the same backend conversation.
  const conversationIdRef = useRef<string | null>(null);
  // Stable uploadBucket for pre-conversation attachments (set by the composer).
  const uploadBucketRef = useRef<string>("");

  // Build the transport once. The refs it reads are consulted inside
  // prepareSendMessagesRequest at send time (not during render), so a stable
  // transport never remounts when the conversation id / upload bucket change.
  /* eslint-disable react-hooks/refs, react-hooks/preserve-manual-memoization */
  const transport = useMemo(
    () =>
      new DefaultChatTransport<DexiaskUIMessage>({
        api: "/api/chat",
        // Send only the new user turn — the backend owns the full history.
        prepareSendMessagesRequest: ({ messages }) => ({
          body: {
            messages: [messages[messages.length - 1]],
            conversationId: conversationIdRef.current ?? undefined,
            uploadBucket: uploadBucketRef.current || undefined,
          },
        }),
      }),
    [],
  );
  /* eslint-enable react-hooks/refs, react-hooks/preserve-manual-memoization */

  const { messages, setMessages, sendMessage, status, stop, regenerate } =
    useChat<DexiaskUIMessage>({
      transport,
      onError: (err) => toast.error(err.message || "Something went wrong"),
    });

  // Reopen a past conversation: on mount, load its transcript and point the ref
  // at it so follow-up turns attach to the same backend conversation. Chat is
  // remounted (keyed) when the selection changes, so this runs once per open.
  useEffect(() => {
    if (!loadConversationId) return;
    conversationIdRef.current = loadConversationId;
    let active = true;
    fetchConversationMessages(loadConversationId)
      .then((msgs) => {
        if (active) setMessages(msgs);
      })
      .catch(() => {
        if (active) toast.error("Failed to load conversation");
      });
    return () => {
      active = false;
    };
  }, [loadConversationId, setMessages]);

  // Capture the backend-assigned conversation id from the first
  // `data-conversation` SSE frame so follow-up turns reference the same
  // conversation. A ref (not state) so it never re-renders the streaming chat.
  // Only fires for a brand-new chat (a reopened one already has the ref set).
  useEffect(() => {
    if (conversationIdRef.current) return;
    for (const msg of messages) {
      for (const part of msg.parts ?? []) {
        if (part.type === "data-conversation") {
          conversationIdRef.current = part.data.id;
          onConversationCreated?.(part.data.id);
          return;
        }
      }
    }
  }, [messages, onConversationCreated]);

  const handleSend = useCallback(
    (text: string, files: FileUIPart[]) => {
      sendMessage({ text, files });
    },
    [sendMessage],
  );

  /**
   * Explicit stop: tell the backend to cancel the run, then stop locally so the
   * UI returns to idle immediately. Fire-and-forget the backend call.
   */
  const handleStop = useCallback(() => {
    const convId = conversationIdRef.current;
    if (convId) {
      fetch(`/api/chat/stop?conversationId=${encodeURIComponent(convId)}`, {
        method: "POST",
      }).catch(() => {
        // Ignore network errors — the local stop still fires below.
      });
    }
    stop();
  }, [stop]);

  const empty = messages.length === 0;
  const canRegenerate =
    status === "ready" && messages[messages.length - 1]?.role === "assistant";

  // Intentionally read during render: the conversation ID lives in a ref so that
  // assigning it never re-renders the streaming chat. The Composer only needs the
  // value as of this render to scope its upload target.
  // eslint-disable-next-line react-hooks/refs
  const composerConversationId = conversationIdRef.current;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        {empty ? (
          <ChatEmptyState
            title="What can I help you understand?"
            description="Dexiask reads your codebase and answers questions with semantic code search."
            suggestions={suggestions}
            onSuggestion={(s) => handleSend(s, [])}
          />
        ) : (
          <MessageList
            messages={messages}
            status={status}
            onAnswer={(text) => sendMessage({ text })}
          />
        )}
      </div>

      <div className="bg-background/80 backdrop-blur">
        <div className="mx-auto w-full max-w-[var(--message-max-width)] px-4 py-3">
          {canRegenerate && (
            <div className="mb-2 flex justify-center">
              <Button
                variant="ghost"
                size="sm"
                className="h-7 gap-1.5 text-xs text-muted-foreground"
                onClick={() => regenerate()}
              >
                <RefreshCw className="h-3.5 w-3.5" />
                Regenerate
              </Button>
            </div>
          )}
          <Composer
            status={status}
            conversationId={composerConversationId}
            onSend={handleSend}
            onStop={handleStop}
            onUploadBucketChange={(bucket) => {
              uploadBucketRef.current = bucket;
            }}
          />
          <p className="mt-2 text-center text-xs text-muted-foreground">
            Dexiask can make mistakes. Verify important output.
          </p>
        </div>
      </div>
    </div>
  );
}
