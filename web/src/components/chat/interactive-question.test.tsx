import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { Message } from "@/components/chat/message";
import type { DexiaskUIMessage } from "@/types/chat";

/**
 * Build an assistant message carrying a single AskChoice tool call. The engine
 * emits it MCP-namespaced (`mcp__interactive__AskChoice`); the UI must reduce
 * that to the bare name and render the question payload as clickable choices.
 */
function askChoiceMessage(): DexiaskUIMessage {
  return {
    id: "a1",
    role: "assistant",
    parts: [
      {
        type: "dynamic-tool",
        toolName: "mcp__interactive__AskChoice",
        toolCallId: "t1",
        state: "input-available",
        input: {
          questions: [
            {
              question: "Which area should I dig into?",
              header: "Focus",
              multiSelect: false,
              options: [
                { label: "Auth flow" },
                { label: "API contracts" },
                { label: "Frontend" },
              ],
            },
          ],
        },
      },
    ],
  } as DexiaskUIMessage;
}

describe("InteractiveQuestion (AskChoice)", () => {
  it("renders the option labels as clickable buttons", () => {
    render(<Message message={askChoiceMessage()} interactive />);
    expect(screen.getByRole("button", { name: /Auth flow/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /API contracts/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Frontend/ })).toBeTruthy();
  });

  it("submits the selection as a composed answer via onAnswer", async () => {
    const onAnswer = vi.fn();
    const user = userEvent.setup();
    render(
      <Message message={askChoiceMessage()} interactive onAnswer={onAnswer} />,
    );

    await user.click(screen.getByRole("button", { name: /Auth flow/ }));
    await user.click(screen.getByRole("button", { name: /^Submit$/ }));

    expect(onAnswer).toHaveBeenCalledTimes(1);
    expect(onAnswer).toHaveBeenCalledWith("- Focus: Auth flow");
  });

  it("does not answer when not interactive (falls back to locked)", () => {
    const onAnswer = vi.fn();
    render(<Message message={askChoiceMessage()} onAnswer={onAnswer} />);
    // Locked: no Submit button is offered when the turn isn't answerable.
    expect(screen.queryByRole("button", { name: /^Submit$/ })).toBeNull();
  });
});
