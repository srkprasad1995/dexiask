"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { MessageSquare, Plus } from "lucide-react";

import { useConversations } from "@/lib/api/conversations";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (!then) return "";
  const secs = Math.max(0, (Date.now() - then) / 1000);
  if (secs < 60) return "now";
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h`;
  const days = Math.floor(hrs / 24);
  if (days < 7) return `${days}d`;
  return new Date(iso).toLocaleDateString();
}

/**
 * Chat history in the app sidebar, below the nav sections. Selecting a
 * conversation opens it in the chat via the `?c=<id>` URL (so it works from any
 * page); "New chat" clears it. Paginated via the backend page_token cursor.
 */
export function ConversationHistory({ onNavigate }: { onNavigate?: () => void }) {
  const router = useRouter();
  const activeId = useSearchParams().get("c");
  const { data, isLoading, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useConversations();

  const conversations = data?.pages.flatMap((p) => p.conversations) ?? [];

  function open(id: string) {
    router.push(`/?c=${encodeURIComponent(id)}`);
    onNavigate?.();
  }
  function newChat() {
    router.push("/");
    onNavigate?.();
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-1 px-1">
      <div className="flex items-center justify-between px-1.5 pt-1">
        <span className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
          Chats
        </span>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          aria-label="New chat"
          onClick={newChat}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isLoading ? (
          <p className="px-2 py-4 text-xs text-muted-foreground">Loading…</p>
        ) : conversations.length === 0 ? (
          <p className="px-2 py-4 text-xs text-muted-foreground">
            No conversations yet.
          </p>
        ) : (
          <nav className="flex flex-col gap-0.5">
            {conversations.map((c) => (
              <button
                key={c.id}
                onClick={() => open(c.id)}
                title={c.title || "Untitled"}
                className={cn(
                  "flex items-center gap-2 rounded-lg px-2 py-1.5 text-left text-[13px] transition-colors",
                  c.id === activeId
                    ? "bg-primary/10 text-primary"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <MessageSquare className="h-3.5 w-3.5 shrink-0" />
                <span className="min-w-0 flex-1 truncate">
                  {c.title || "Untitled"}
                </span>
                <span className="shrink-0 text-[10px] tabular-nums opacity-60">
                  {relativeTime(c.updated_at)}
                </span>
              </button>
            ))}
            {hasNextPage && (
              <button
                onClick={() => fetchNextPage()}
                disabled={isFetchingNextPage}
                className="mt-1 rounded-lg px-2 py-1.5 text-left text-xs text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
              >
                {isFetchingNextPage ? "Loading…" : "Load more"}
              </button>
            )}
          </nav>
        )}
      </div>
    </div>
  );
}
