"use client";

import { Plus, MessageSquare } from "lucide-react";

import { useConversations } from "@/lib/api/conversations";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (!then) return "";
  const secs = Math.max(0, (Date.now() - then) / 1000);
  if (secs < 60) return "just now";
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

/**
 * Chat history rail: a "New chat" button and the user's past conversations,
 * most-recent first. Selecting one reopens its transcript.
 */
export function ConversationSidebar({
  selectedId,
  onSelect,
  onNew,
}: {
  selectedId: string | null;
  onSelect: (id: string) => void;
  onNew: () => void;
}) {
  const { data, isLoading } = useConversations();
  const conversations = data ?? [];

  return (
    <div className="flex h-full flex-col gap-2 p-2">
      <Button
        variant="outline"
        className="w-full justify-start gap-2"
        onClick={onNew}
      >
        <Plus className="h-4 w-4" />
        New chat
      </Button>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isLoading ? (
          <div className="space-y-1.5 p-1">
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
          </div>
        ) : conversations.length === 0 ? (
          <p className="px-2 py-6 text-center text-xs text-muted-foreground">
            No conversations yet.
          </p>
        ) : (
          <nav className="flex flex-col gap-0.5">
            {conversations.map((c) => (
              <button
                key={c.id}
                onClick={() => onSelect(c.id)}
                title={c.title || "Untitled"}
                className={cn(
                  "flex items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors",
                  c.id === selectedId
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
          </nav>
        )}
      </div>
    </div>
  );
}
