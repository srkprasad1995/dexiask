import { SpanStatusCode, trace } from "@opentelemetry/api";
import type { UIMessageStreamWriter } from "ai";

import type { DexiaskUIMessage } from "@/types/chat";

const tracer = trace.getTracer("dexiask-web");

/**
 * Translator: Dexiask Go SSE  ->  AI SDK v6 UI message stream.
 *
 * This is the linchpin of the BFF. The Go backend owns LLM/agent orchestration
 * and emits its OWN SSE envelope; we keep that coupling here (server-side) so
 * the browser stays on the default AI SDK transport. When the Go event schema
 * changes, this file is the single place to update. The event `type` strings
 * below MUST match exactly what the Go backend emits.
 *
 * Expected Go event shape (one JSON object per SSE `data:` line):
 *   { "type": "text.delta",   "id": "b1", "text": "..." }
 *   { "type": "text.start"|"text.stop", "id": "b1" }
 *   { "type": "thinking.delta", "id": "r1", "text": "..." }
 *   { "type": "tool.start",   "id": "t1", "name": "search_codebase" }
 *   { "type": "tool.input_delta", "id": "t1", "json": "{\"q\":" }
 *   { "type": "tool.input_done", "id": "t1", "name": "...", "input": {...} }
 *   { "type": "tool.result",  "id": "t1", "result": {...} }
 *   { "type": "conversation", "id": "conv-123" }
 *   { "type": "error",        "message": "..." }
 */

type Writer = UIMessageStreamWriter<DexiaskUIMessage>;

interface GoEvent {
  type: string;
  id?: string;
  name?: string;
  text?: string;
  json?: string;
  input?: unknown;
  result?: unknown;
  message?: string;
}

/** Parse a byte stream of SSE frames into decoded JSON events. `onDropped` is
 * called once per malformed frame that is skipped. */
export async function* parseGoSSE(
  body: ReadableStream<Uint8Array>,
  onDropped?: () => void,
): AsyncGenerator<GoEvent> {
  const textStream = body.pipeThrough(
    new TextDecoderStream() as unknown as ReadableWritablePair<
      string,
      Uint8Array
    >,
  );
  const reader = textStream.getReader();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += value;

      let sep: number;
      // SSE frames are separated by a blank line.
      while ((sep = buffer.indexOf("\n\n")) !== -1) {
        const frame = buffer.slice(0, sep);
        buffer = buffer.slice(sep + 2);

        const dataLines = frame
          .split("\n")
          .filter((l) => l.startsWith("data:"))
          .map((l) => l.slice(5).trim());
        if (dataLines.length === 0) continue;

        const payload = dataLines.join("\n");
        if (payload === "[DONE]") return;
        try {
          yield JSON.parse(payload) as GoEvent;
        } catch {
          // Skip malformed frames rather than killing the stream.
          onDropped?.();
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

/** Pump a parsed Go event stream into the AI SDK writer. */
export async function pumpGoStream(
  body: ReadableStream<Uint8Array>,
  writer: Writer,
) {
  // Accumulate per-tool state so tool-input-available and tool-output-available
  // always have required fields even when the engine omits them.
  const toolNames = new Map<string, string>();
  const toolInputBuffers = new Map<string, string>();

  // Track which text/reasoning parts are currently open in the UI stream. The
  // AI SDK rejects a `text-end`/`reasoning-end` for a part it never saw start
  // (throws "Received text-end for missing text part…"). Upstream ordering can
  // break the pairing — e.g. a dropped `text.start` frame, or an engine that
  // emits a stop without a start — so we enforce well-formed
  // start→(delta)*→end sequences here, the single translation point, rather
  // than trusting every engine to be perfectly paired.
  const openText = new Set<string>();
  const openReasoning = new Set<string>();

  // Span the whole SSE translation so the BFF's piece of the trace is visible,
  // with frame/error/dropped counts as attributes.
  const span = tracer.startSpan("chat.sse_pump");
  let frames = 0;
  let dropped = 0;
  let sawError = false;

  try {
    for await (const ev of parseGoSSE(body, () => (dropped += 1))) {
      frames += 1;
      switch (ev.type) {
        case "text.start":
          if (!openText.has(ev.id!)) {
            openText.add(ev.id!);
            writer.write({ type: "text-start", id: ev.id! });
          }
          break;
        case "text.delta":
          // A delta implies the part is live — open it if the start was lost.
          if (!openText.has(ev.id!)) {
            openText.add(ev.id!);
            writer.write({ type: "text-start", id: ev.id! });
          }
          writer.write({
            type: "text-delta",
            id: ev.id!,
            delta: ev.text ?? "",
          });
          break;
        case "text.stop":
          // Only close a part the UI stream actually opened; drop stray stops.
          if (openText.delete(ev.id!)) {
            writer.write({ type: "text-end", id: ev.id! });
          }
          break;

        case "thinking.start":
          if (!openReasoning.has(ev.id!)) {
            openReasoning.add(ev.id!);
            writer.write({ type: "reasoning-start", id: ev.id! });
          }
          break;
        case "thinking.delta":
          if (!openReasoning.has(ev.id!)) {
            openReasoning.add(ev.id!);
            writer.write({ type: "reasoning-start", id: ev.id! });
          }
          writer.write({
            type: "reasoning-delta",
            id: ev.id!,
            delta: ev.text ?? "",
          });
          break;
        case "thinking.stop":
          if (openReasoning.delete(ev.id!)) {
            writer.write({ type: "reasoning-end", id: ev.id! });
          }
          break;

        case "tool.start": {
          const name = ev.name ?? "tool";
          toolNames.set(ev.id!, name);
          toolInputBuffers.set(ev.id!, "");
          writer.write({
            type: "tool-input-start",
            toolCallId: ev.id!,
            toolName: name,
          });
          break;
        }
        case "tool.input_delta": {
          toolInputBuffers.set(
            ev.id!,
            (toolInputBuffers.get(ev.id!) ?? "") + (ev.json ?? ""),
          );
          writer.write({
            type: "tool-input-delta",
            toolCallId: ev.id!,
            inputTextDelta: ev.json ?? "",
          });
          break;
        }
        case "tool.input_done": {
          const rawJson = toolInputBuffers.get(ev.id!) ?? "{}";
          toolInputBuffers.delete(ev.id!);
          let input: unknown = {};
          try {
            input = JSON.parse(rawJson);
          } catch {
            /* keep {} */
          }
          const name = ev.name ?? toolNames.get(ev.id!) ?? "tool";
          writer.write({
            type: "tool-input-available",
            toolCallId: ev.id!,
            toolName: name,
            input,
          });
          break;
        }
        case "tool.result": {
          // result field populated by the engine (via its tool_result event).
          const output = ev.result ?? "";
          toolNames.delete(ev.id!);
          writer.write({
            type: "tool-output-available",
            toolCallId: ev.id!,
            output,
          });
          break;
        }

        case "conversation":
          // First-frame event: the backend-assigned conversation ID.
          // Captured by chat.tsx to update the URL and the outgoing-request ref.
          writer.write({
            type: "data-conversation",
            data: { id: ev.id! },
          });
          break;

        case "error":
          sawError = true;
          span.addEvent("stream.error", {
            message: ev.message ?? "stream error",
          });
          writer.write({
            type: "error",
            errorText: ev.message ?? "stream error",
          });
          break;
      }
    }
  } catch (err) {
    sawError = true;
    span.recordException(err as Error);
    throw err;
  } finally {
    span.setAttribute("sse.frames", frames);
    span.setAttribute("sse.dropped_frames", dropped);
    if (sawError) span.setStatus({ code: SpanStatusCode.ERROR });
    span.end();
  }
}
