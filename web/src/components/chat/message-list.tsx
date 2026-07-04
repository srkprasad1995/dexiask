"use client";

import { useEffect, useMemo, useRef } from "react";
import type { ChatStatus } from "ai";

import type { DexiaskUIMessage } from "@/types/chat";
import { Message } from "@/components/chat/message";
import { ProcessingIndicator } from "@/components/chat/processing-indicator";

/**
 * Returns true when the streaming assistant message has not yet produced any
 * visible text — i.e., the indicator should remain visible. Pre-text events
 * (reasoning, agent steps, tool calls) still render inline inside the message
 * bubble; we want the placeholder to persist until prose actually starts.
 */
function hasNoTextYet(messages: DexiaskUIMessage[]): boolean {
  const last = messages[messages.length - 1];
  if (!last || last.role !== "assistant") return true;
  return !last.parts.some(
    (part) =>
      part.type === "text" &&
      (part as { type: "text"; text: string }).text.length > 0,
  );
}

export function MessageList({
  messages,
  status,
  onAnswer,
}: {
  messages: DexiaskUIMessage[];
  status: ChatStatus;
  /** Send an AskChoice selection back as the next user turn. */
  onAnswer?: (text: string) => void;
}) {
  const bottomRef = useRef<HTMLDivElement>(null);
  // Whether to keep pinning the view to the latest content. True while the user
  // is near the bottom; flips false as soon as they scroll up to read. Without
  // this, the auto-scroll effect re-fires on every `messages`/`status` change
  // (incl. resume churn + async diagram layout) and yanks the viewport — which
  // reads as heavy flicker on long, diagram-rich conversations.
  const stickRef = useRef(true);

  useEffect(() => {
    // Find the nearest scrollable ancestor (the chat scroll container).
    let scroller: HTMLElement | null = bottomRef.current?.parentElement ?? null;
    while (scroller) {
      const oy = getComputedStyle(scroller).overflowY;
      if (oy === "auto" || oy === "scroll") break;
      scroller = scroller.parentElement;
    }
    if (!scroller) return;
    const el = scroller;
    // Every chat opens pinned to the latest message. Jump to the bottom on mount
    // BEFORE wiring the scroll listener — otherwise the first onScroll sees the
    // fresh scrollTop=0 on a tall transcript, marks us "not stuck", and the
    // auto-follow never engages (chats opened scrolled to the top).
    el.scrollTop = el.scrollHeight;
    stickRef.current = true;
    const onScroll = () => {
      const gap = el.scrollHeight - el.scrollTop - el.clientHeight;
      stickRef.current = gap < 120; // within ~120px of the bottom = "stuck"
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  // Belt-and-suspenders: after the first paint (and any late layout from async
  // markdown/diagrams), snap to the bottom once more so the initial view lands
  // on the latest message even when content grows after mount.
  useEffect(() => {
    const t = setTimeout(
      () => bottomRef.current?.scrollIntoView({ block: "end" }),
      50,
    );
    return () => clearTimeout(t);
  }, []);

  // Holistic de-duplication of the well-known engine behavior where one logical
  // turn arrives as several assistant messages with cumulative/repeated text.
  const visibleMessages = useMemo(() => {
    // 1) Drop exact duplicate ids (a streamed message that briefly appears twice
    //    during resume/regenerate) — keep the last (freshest) occurrence.
    const seen = new Set<string>();
    const byId: DexiaskUIMessage[] = [];
    for (let i = messages.length - 1; i >= 0; i--) {
      if (seen.has(messages[i].id)) continue;
      seen.add(messages[i].id);
      byId.push(messages[i]);
    }
    byId.reverse();

    // 2) Drop an assistant message whose prose is fully contained at the start of
    //    a LATER assistant message's prose — the engine re-emits accumulated text
    //    across agent steps, so the later message already supersedes it. Empty /
    //    tool-only messages (no prose) are always kept.
    const proseOf = (m: DexiaskUIMessage) =>
      m.parts
        .filter((p) => p.type === "text")
        .map((p) => (p.type === "text" ? p.text : ""))
        .join("")
        .trim();
    const prose = byId.map(proseOf);
    return byId.filter((m, i) => {
      if (m.role !== "assistant" || !prose[i]) return true;
      for (let j = i + 1; j < byId.length; j++) {
        if (byId[j].role === "assistant" && prose[j].startsWith(prose[i]))
          return false;
      }
      return true;
    });
  }, [messages]);

  // Follow the latest content only while the user is pinned to the bottom, so
  // scrolling up to read isn't interrupted.
  useEffect(() => {
    if (!stickRef.current) return;
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [messages, status]);

  const last = visibleMessages[visibleMessages.length - 1];

  // Show the animated placeholder:
  //  • while submitted (message sent, awaiting first backend event)
  //  • while streaming but before any assistant text has arrived (pre-text phase:
  //    reasoning, agent steps, tool calls may be rendering, but the prose hasn't
  //    started yet and the indicator anchors the bottom of the list)
  const showIndicator =
    status === "submitted" ||
    (status === "streaming" && last?.role === "user") ||
    (status === "streaming" && hasNoTextYet(messages));

  return (
    <div className="mx-auto w-full max-w-[var(--message-max-width)] space-y-6 px-4 py-6">
      {visibleMessages.map((message) => (
        <Message
          key={message.id}
          message={message}
          interactive={status === "ready" && message.id === last?.id}
          streaming={
            (status === "streaming" || status === "submitted") &&
            message.id === last?.id
          }
          onAnswer={onAnswer}
        />
      ))}
      {showIndicator && <ProcessingIndicator />}
      <div ref={bottomRef} />
    </div>
  );
}
