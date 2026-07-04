import type { ReactNode } from "react";

/**
 * The single empty-state layout shared by every chat surface (Ask + project
 * stage chat), so they read identically: a vertically-centered column with an
 * optional mono eyebrow, a title, a muted description, an optional primary CTA,
 * and optional suggestion chips. Only the content differs per surface.
 */
export function ChatEmptyState({
  eyebrow,
  title,
  description,
  action,
  suggestions = [],
  onSuggestion,
}: {
  eyebrow?: string;
  title: string;
  description?: string;
  action?: ReactNode;
  suggestions?: string[];
  onSuggestion?: (s: string) => void;
}) {
  return (
    <div className="mx-auto flex h-full max-w-[var(--message-max-width)] flex-col items-center justify-center gap-6 px-4 text-center">
      <div className="space-y-2">
        {eyebrow && (
          <p className="font-plex-mono text-[10px] tracking-[0.2em] text-muted-foreground uppercase">
            {eyebrow}
          </p>
        )}
        <h1 className="text-lg font-semibold tracking-tight">{title}</h1>
        {description && (
          <p className="mx-auto max-w-md text-sm text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {action}
      {suggestions.length > 0 && (
        <div className="flex flex-wrap justify-center gap-2">
          {suggestions.map((s) => (
            <button
              key={s}
              onClick={() => onSuggestion?.(s)}
              className="rounded-full border bg-muted/50 px-3.5 py-1.5 text-sm transition-colors hover:border-primary/30 hover:bg-muted"
            >
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
