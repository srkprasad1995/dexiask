import { describe, it, expect, vi, beforeAll } from "vitest";
import { TransformStream, TextDecoderStream } from "node:stream/web";
import type { UIMessageStreamWriter } from "ai";

import { parseGoSSE, pumpGoStream } from "@/lib/chat/go-sse-to-ui";
import type { DexiaskUIMessage } from "@/types/chat";

// Polyfill web streams for happy-dom if not available.
beforeAll(() => {
  const g = globalThis as Record<string, unknown>;
  if (!g["TextDecoderStream"]) g["TextDecoderStream"] = TextDecoderStream;
  if (!g["TransformStream"]) g["TransformStream"] = TransformStream;
});

type Writer = UIMessageStreamWriter<DexiaskUIMessage>;

function makeByteStream(text: string): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(text));
      controller.close();
    },
  });
}

function makeEventStream(events: object[]): ReadableStream<Uint8Array> {
  return makeByteStream(
    events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join(""),
  );
}

function makeWriter() {
  const calls: unknown[] = [];
  const writer = {
    write: vi.fn((chunk: unknown) => {
      calls.push(chunk);
    }),
  } as unknown as Writer;
  return { writer, calls };
}

async function collectEvents(stream: ReadableStream<Uint8Array>) {
  const events: unknown[] = [];
  for await (const ev of parseGoSSE(stream)) events.push(ev);
  return events;
}

describe("parseGoSSE", () => {
  it("yields no events for an empty stream", async () => {
    expect(await collectEvents(makeByteStream(""))).toEqual([]);
  });

  it("parses a single frame into one event", async () => {
    const events = await collectEvents(
      makeByteStream(`data: {"type":"text.delta","id":"b1","text":"hello"}\n\n`),
    );
    expect(events).toEqual([{ type: "text.delta", id: "b1", text: "hello" }]);
  });

  it("stops at [DONE]", async () => {
    const events = await collectEvents(
      makeByteStream(
        `data: {"type":"text.delta","id":"b1","text":"before"}\n\n` +
          `data: [DONE]\n\n` +
          `data: {"type":"text.delta","id":"b1","text":"after"}\n\n`,
      ),
    );
    expect(events).toHaveLength(1);
  });

  it("skips malformed JSON frames without throwing", async () => {
    const events = await collectEvents(
      makeByteStream(
        `data: {"type":"text.start","id":"b1"}\n\n` +
          `data: not-json{\n\n` +
          `data: {"type":"text.stop","id":"b1"}\n\n`,
      ),
    );
    expect(events).toHaveLength(2);
  });

  it("joins multiple data: lines in one frame", async () => {
    const events = await collectEvents(
      makeByteStream(
        `data: {"type":"text.delta","id":"b1",\ndata: "text":"split"}\n\n`,
      ),
    );
    expect(events).toEqual([{ type: "text.delta", id: "b1", text: "split" }]);
  });
});

describe("pumpGoStream — event mapping", () => {
  it("maps text.start/delta/stop", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "text.start", id: "b1" },
        { type: "text.delta", id: "b1", text: "hi" },
        { type: "text.stop", id: "b1" },
      ]),
      writer,
    );
    expect(calls).toEqual([
      { type: "text-start", id: "b1" },
      { type: "text-delta", id: "b1", delta: "hi" },
      { type: "text-end", id: "b1" },
    ]);
  });

  it("maps thinking.* to reasoning-*", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "thinking.start", id: "r1" },
        { type: "thinking.delta", id: "r1", text: "hmm" },
        { type: "thinking.stop", id: "r1" },
      ]),
      writer,
    );
    expect(calls).toEqual([
      { type: "reasoning-start", id: "r1" },
      { type: "reasoning-delta", id: "r1", delta: "hmm" },
      { type: "reasoning-end", id: "r1" },
    ]);
  });

  it("maps a full tool lifecycle with parsed input + result", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "tool.start", id: "t1", name: "search_codebase" },
        { type: "tool.input_delta", id: "t1", json: '{"q":"foo"}' },
        { type: "tool.input_done", id: "t1", name: "search_codebase" },
        { type: "tool.result", id: "t1", result: { files: ["a.ts"] } },
      ]),
      writer,
    );
    expect(calls).toContainEqual({
      type: "tool-input-start",
      toolCallId: "t1",
      toolName: "search_codebase",
    });
    expect(calls).toContainEqual(
      expect.objectContaining({
        type: "tool-input-available",
        toolCallId: "t1",
        toolName: "search_codebase",
        input: { q: "foo" },
      }),
    );
    expect(calls).toContainEqual(
      expect.objectContaining({
        type: "tool-output-available",
        toolCallId: "t1",
        output: { files: ["a.ts"] },
      }),
    );
  });

  it("maps conversation to data-conversation", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([{ type: "conversation", id: "conv-123" }]),
      writer,
    );
    expect(calls).toContainEqual({
      type: "data-conversation",
      data: { id: "conv-123" },
    });
  });

  it("maps error, with a fallback message", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([{ type: "error", message: "boom" }, { type: "error" }]),
      writer,
    );
    expect(calls).toContainEqual({ type: "error", errorText: "boom" });
    expect(calls).toContainEqual({ type: "error", errorText: "stream error" });
  });

  it("silently ignores unknown/removed event types", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "project.stage_transition", project: {} },
        { type: "agent.step", step: {} },
        { type: "unknown.type", id: "x" },
      ]),
      writer,
    );
    expect(calls).toHaveLength(0);
  });
});

// The AI SDK throws "Received text-end for missing text part…" if an end reaches
// the client without a matching start. The pump guarantees well-formed pairing.
describe("pumpGoStream — start/end pairing guard", () => {
  it("drops a text.stop with no preceding text.start", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(makeEventStream([{ type: "text.stop", id: "b3" }]), writer);
    expect(calls).toHaveLength(0);
  });

  it("drops a thinking.stop with no preceding thinking.start", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([{ type: "thinking.stop", id: "r1" }]),
      writer,
    );
    expect(calls).toHaveLength(0);
  });

  it("synthesizes a text-start when a delta arrives without one", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "text.delta", id: "b3", text: "hi" },
        { type: "text.stop", id: "b3" },
      ]),
      writer,
    );
    expect(calls).toEqual([
      { type: "text-start", id: "b3" },
      { type: "text-delta", id: "b3", delta: "hi" },
      { type: "text-end", id: "b3" },
    ]);
  });

  it("synthesizes a reasoning-start when a delta arrives without one", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "thinking.delta", id: "r1", text: "hmm" },
        { type: "thinking.stop", id: "r1" },
      ]),
      writer,
    );
    expect(calls).toEqual([
      { type: "reasoning-start", id: "r1" },
      { type: "reasoning-delta", id: "r1", delta: "hmm" },
      { type: "reasoning-end", id: "r1" },
    ]);
  });

  it("does not emit a duplicate text-start on repeated text.start", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "text.start", id: "b3" },
        { type: "text.start", id: "b3" },
      ]),
      writer,
    );
    expect(
      calls.filter((c) => (c as { type?: string }).type === "text-start"),
    ).toHaveLength(1);
  });

  it("drops a second text.stop for an already-closed part", async () => {
    const { writer, calls } = makeWriter();
    await pumpGoStream(
      makeEventStream([
        { type: "text.start", id: "b3" },
        { type: "text.stop", id: "b3" },
        { type: "text.stop", id: "b3" },
      ]),
      writer,
    );
    expect(
      calls.filter((c) => (c as { type?: string }).type === "text-end"),
    ).toHaveLength(1);
  });
});
