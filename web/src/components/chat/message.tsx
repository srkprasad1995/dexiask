"use client";

import { memo, useEffect, useRef, useState, type ReactNode } from "react";
import { isToolOrDynamicToolUIPart } from "ai";
import type { FileUIPart } from "ai";
import { Brain, ChevronRight } from "lucide-react";

import type { DexiaskUIMessage } from "@/types/chat";
import { DexiaskAvatar } from "@/components/brand/dexiask-avatar";
import { Markdown } from "@/components/chat/markdown";
import { Reasoning } from "@/components/chat/reasoning";
import { ToolCall } from "@/components/chat/tool-call";
import { Attachment } from "@/components/chat/attachment";
import { InteractiveQuestion } from "@/components/chat/interactive-question";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";

/**
 * Detect the structured answer InteractiveQuestion sends back — lines shaped
 * `- Header: a, b, c`. When every line matches, we render it as compact chips
 * instead of a cramped wall of text in the user bubble. Returns null otherwise.
 */
function parseChoiceAnswer(
  text: string,
): { header: string; values: string[] }[] | null {
  const lines = text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);
  if (lines.length === 0) return null;
  const items: { header: string; values: string[] }[] = [];
  for (const line of lines) {
    const m = /^-\s+(.+?):\s*(.+)$/.exec(line);
    if (!m) return null;
    items.push({
      header: m[1].trim(),
      values: m[2]
        .split(",")
        .map((v) => v.trim())
        .filter(Boolean),
    });
  }
  return items.length ? items : null;
}

/** Renders a submitted AskChoice answer as a right-aligned chip summary. */
function ChoiceAnswer({
  items,
}: {
  items: { header: string; values: string[] }[];
}) {
  return (
    <div className="max-w-[85%] space-y-2 rounded-2xl rounded-br-sm border border-primary/30 bg-primary/5 px-4 py-3">
      {items.map((it, i) => (
        <div key={i} className="flex flex-wrap items-center justify-end gap-1.5">
          <span className="font-plex-mono text-[10px] tracking-wide text-muted-foreground uppercase">
            {it.header}
          </span>
          {it.values.map((v, j) => (
            <span
              key={j}
              className="rounded-full bg-primary/15 px-2 py-0.5 text-xs font-medium text-primary"
            >
              {v}
            </span>
          ))}
        </div>
      ))}
    </div>
  );
}

/**
 * Collapses a turn's process — reasoning + every tool call — into ONE
 * "thinking" section. Auto-expands while the turn streams (live progress), then
 * auto-collapses on completion so a finished turn shows only the final answer.
 */
function WorkGroup({
  count,
  streaming,
  children,
}: {
  count: number;
  streaming: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(streaming);
  const prev = useRef(streaming);
  useEffect(() => {
    if (prev.current !== streaming) {
      setOpen(streaming);
      prev.current = streaming;
    }
  }, [streaming]);

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="my-2 overflow-hidden rounded-md border bg-card shadow-dx-sm"
    >
      <CollapsibleTrigger className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-muted-foreground transition-colors hover:text-foreground">
        <Brain className="h-3.5 w-3.5 shrink-0" />
        <span>
          {streaming
            ? "Working…"
            : `Worked · ${count} ${count === 1 ? "step" : "steps"}`}
        </span>
        <ChevronRight
          className={cn(
            "ml-auto h-3.5 w-3.5 shrink-0 transition-transform",
            open && "rotate-90",
          )}
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="space-y-1 px-2 pb-2">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * The engine re-emits text in blocks across agent steps, often cumulatively, so
 * the same response arrives 2-3 times. Keep only blocks that aren't superseded
 * by a LATER block (one whose text starts with this one) — collapsing cumulative
 * duplicates to the single final answer while preserving genuinely distinct
 * sequential blocks.
 */
function dedupeTexts(texts: string[]): string[] {
  const nonEmpty = texts.filter((t) => t.trim());
  return nonEmpty.filter((t, i) => {
    const cur = t.trim();
    return !nonEmpty.slice(i + 1).some((later) => later.trim().startsWith(cur));
  });
}

/** Strip the `mcp__<server>__` namespace from a dynamic tool name. */
function bareToolName(name: string): string {
  return name.startsWith("mcp__")
    ? name.split("__").slice(2).join("__") || name
    : name;
}

type MessagePart = DexiaskUIMessage["parts"][number];

/** Bare tool name of a tool/dynamic-tool part. */
function partToolName(part: MessagePart): string {
  if (!isToolOrDynamicToolUIPart(part)) return "";
  const raw =
    part.type === "dynamic-tool"
      ? part.toolName
      : part.type.replace(/^tool-/, "");
  return bareToolName(raw);
}

/** True for an AskChoice tool part in either `dynamic-tool` or `tool-<name>` form. */
function isAskChoicePart(part: MessagePart): boolean {
  return partToolName(part) === "AskChoice";
}

/**
 * Renders one message by iterating its `parts` (the AI SDK v6 model). Memoized
 * so completed messages never re-render while a later message streams.
 */
export const Message = memo(function Message({
  message,
  interactive = false,
  streaming = false,
  onAnswer,
}: {
  message: DexiaskUIMessage;
  /** True when this is the latest message and the chat is idle — its
   * AskChoice question (if any) is answerable. */
  interactive?: boolean;
  /** True while this is the actively-streaming turn — keeps the work section
   * expanded until the turn completes, then it auto-collapses. */
  streaming?: boolean;
  /** Send the user's answer back as the next user turn. */
  onAnswer?: (text: string) => void;
}) {
  if (message.role === "user") {
    const fileParts = message.parts.filter(
      (p): p is FileUIPart => p.type === "file",
    );
    const text = message.parts
      .filter((p) => p.type === "text")
      .map((p) => (p.type === "text" ? p.text : ""))
      .join("");
    const choiceAnswer = text ? parseChoiceAnswer(text) : null;
    return (
      <div className="flex flex-col items-end gap-2">
        {fileParts.length > 0 && (
          <div className="flex flex-wrap justify-end gap-2">
            {fileParts.map((p, i) => (
              <Attachment key={i} part={p} />
            ))}
          </div>
        )}
        {text &&
          (choiceAnswer ? (
            <ChoiceAnswer items={choiceAnswer} />
          ) : (
            <div className="max-w-[80%] rounded-2xl rounded-br-sm bg-primary px-4 py-2.5 text-sm whitespace-pre-wrap text-primary-foreground">
              {text}
            </div>
          ))}
      </div>
    );
  }

  // Partition the assistant turn into two lanes so it renders as ONE clean unit
  // instead of many interleaved cards:
  //  • work  — reasoning + every tool call (collapsed into one section)
  //  • texts — prose answer blocks (deduped to the final message)
  //  • tail  — interactive / side-effecting bits that must stay visible:
  //            file attachments and the combined AskChoice card.
  const work: ReactNode[] = [];
  const texts: string[] = [];
  const tail: ReactNode[] = [];
  // AskChoice is combined into a single card (the model may split questions
  // across calls) rendered in the tail, never as a work/tool card.
  const askChoiceInputs: unknown[] = [];

  message.parts.forEach((part: MessagePart, i) => {
    if (part.type === "text") {
      if (part.text.trim()) texts.push(part.text);
      return;
    }
    if (part.type === "reasoning") {
      work.push(
        <Reasoning
          key={i}
          text={part.text}
          streaming={part.state === "streaming"}
        />,
      );
      return;
    }
    if (part.type === "file") {
      tail.push(<Attachment key={i} part={part as FileUIPart} />);
      return;
    }
    if (isToolOrDynamicToolUIPart(part)) {
      if (isAskChoicePart(part)) {
        askChoiceInputs.push("input" in part ? part.input : undefined);
        return;
      }
      work.push(<ToolCall key={i} part={part} />);
    }
  });

  if (askChoiceInputs.length > 0) {
    tail.push(
      <InteractiveQuestion
        key="askchoice"
        inputs={askChoiceInputs}
        interactive={interactive}
        onAnswer={onAnswer}
      />,
    );
  }

  const finalTexts = dedupeTexts(texts);

  return (
    <div className="flex gap-3">
      <DexiaskAvatar size={28} className="mt-0.5" />
      <div className="min-w-0 flex-1 space-y-1 overflow-hidden pt-0.5">
        {/* All reasoning + tool calls collapsed into one thinking section. */}
        {work.length > 0 && (
          <WorkGroup count={work.length} streaming={streaming}>
            {work}
          </WorkGroup>
        )}
        {/* The final answer (cumulative duplicates dropped). No overflow-clip on
            this tall wrapper — a rounded clip forces re-rasterizing big Mermaid
            SVGs every scroll frame (the long-reply flicker). */}
        {finalTexts.map((t, i) => (
          <div
            key={`text-${i}`}
            className="rounded-[4px_14px_14px_14px] border bg-card px-4 py-3 shadow-dx-sm"
          >
            <Markdown>{t}</Markdown>
          </div>
        ))}
        {tail}
      </div>
    </div>
  );
});
