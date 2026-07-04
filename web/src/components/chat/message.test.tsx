import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { Message } from "@/components/chat/message";
import type { DexiaskUIMessage } from "@/types/chat";

function userMessage(text: string): DexiaskUIMessage {
  return { id: "u1", role: "user", parts: [{ type: "text", text }] };
}

function assistantMessage(text: string): DexiaskUIMessage {
  return { id: "a1", role: "assistant", parts: [{ type: "text", text }] };
}

describe("Message", () => {
  it("renders a user message as its text", () => {
    render(<Message message={userMessage("hello there")} />);
    expect(screen.getByText("hello there")).toBeTruthy();
  });

  it("renders an assistant message's markdown prose", () => {
    render(<Message message={assistantMessage("Hello **world**")} />);
    // Streamdown renders the text; the bold token becomes its own element, so
    // assert the words are present rather than the whole string.
    expect(screen.getByText(/Hello/)).toBeTruthy();
    expect(screen.getByText("world")).toBeTruthy();
  });

  it("renders the Dexiask avatar for assistant turns", () => {
    render(<Message message={assistantMessage("hi")} />);
    expect(screen.getByLabelText("Dexiask")).toBeTruthy();
  });

  it("does not render an avatar for user turns", () => {
    render(<Message message={userMessage("hi")} />);
    expect(screen.queryByLabelText("Dexiask")).toBeNull();
  });
});
