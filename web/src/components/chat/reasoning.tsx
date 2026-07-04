"use client";

import { useEffect, useRef, useState } from "react";
import { Brain, ChevronRight } from "lucide-react";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import { Markdown } from "@/components/chat/markdown";

/**
 * Collapsed-by-default "thinking" trace. Agent reasoning gets long, so we
 * keep it tucked away but available.
 */
export function Reasoning({
  text,
  streaming,
}: {
  text: string;
  streaming?: boolean;
}) {
  // Expanded while the model is actively thinking (live progress), then
  // auto-collapses the moment the reasoning completes. A manual toggle still
  // wins until the next streaming transition.
  const [open, setOpen] = useState(!!streaming);
  const prevStreaming = useRef(!!streaming);
  useEffect(() => {
    if (prevStreaming.current !== !!streaming) {
      setOpen(!!streaming);
      prevStreaming.current = !!streaming;
    }
  }, [streaming]);

  if (!text.trim()) return null;

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="my-2 overflow-hidden rounded-md border bg-card shadow-dx-sm"
    >
      <CollapsibleTrigger className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-muted-foreground transition-colors hover:text-foreground">
        <Brain className="h-3.5 w-3.5 shrink-0" />
        <span>{streaming ? "Thinking…" : "Reasoned"}</span>
        <ChevronRight
          className={cn(
            "ml-auto h-3.5 w-3.5 shrink-0 transition-transform",
            open && "rotate-90",
          )}
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="px-3 pb-3 pl-9 text-sm text-muted-foreground">
          <Markdown>{text}</Markdown>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
