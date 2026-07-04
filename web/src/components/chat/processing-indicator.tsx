"use client";

import { cn } from "@/lib/utils";

/**
 * Animated "being processed" indicator shown from the moment a message is sent
 * until the first assistant text starts streaming. Three bouncing dots with a
 * label; pure CSS animation, no extra dependencies.
 */
export function ProcessingIndicator({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 py-1 text-sm text-muted-foreground",
        className,
      )}
    >
      <span>Dexiask is working</span>
      <span className="flex items-center gap-1">
        <span className="size-1.5 animate-pulse rounded-full bg-muted-foreground [animation-delay:0ms] [animation-duration:1.2s]" />
        <span className="size-1.5 animate-pulse rounded-full bg-muted-foreground [animation-delay:200ms] [animation-duration:1.2s]" />
        <span className="size-1.5 animate-pulse rounded-full bg-muted-foreground [animation-delay:400ms] [animation-duration:1.2s]" />
      </span>
    </div>
  );
}
