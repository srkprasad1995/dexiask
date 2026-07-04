"""
Interactive question capability: AskChoice.

Lets the agent ask the user a structured question (single-select, multi-select,
or yes/no confirm) that the web UI renders as clickable choices, instead of
asking in prose and parsing a free-text reply.

Design — "ask-then-resume" (no blocking): the question payload travels in the
*tool input*, which the orchestrator already streams to the UI end-to-end
(``tool.start`` / ``tool.input_done``). The handler emits nothing extra and does
NOT block; it returns a short ack so the model ends its turn cleanly. The user's
selection arrives as the *next* user turn and the runtime resumes the session via
its session id — so the resumed session is well-formed (no dangling tool_use).

Named ``AskChoice`` — NOT ``AskUserQuestion`` — to avoid colliding with the
built-in Claude Code tool that requires an interactive permission prompt.
"""
from __future__ import annotations

from typing import Any

from ..tools.descriptors import ToolDescriptor

# Returned to the model after it presents a question. Steers it to stop and wait.
_ACK = (
    "Question(s) presented to the user. End your turn now and wait — their "
    "answer will arrive as the next message."
)

ASK_CHOICE_SCHEMA = {
    "type": "object",
    "properties": {
        "questions": {
            "type": "array",
            "minItems": 1,
            "items": {
                "type": "object",
                "properties": {
                    "question": {
                        "type": "string",
                        "description": "The full question to ask the user.",
                    },
                    "header": {
                        "type": "string",
                        "description": "Short label for the question (a chip/tag, "
                        "max ~12 chars), e.g. 'Database', 'Approach'.",
                    },
                    "multiSelect": {
                        "type": "boolean",
                        "description": "True to let the user pick multiple options; "
                        "false for a single choice (a yes/no confirm is just two "
                        "single-select options).",
                    },
                    "options": {
                        "type": "array",
                        "minItems": 2,
                        "items": {
                            "type": "object",
                            "properties": {
                                "label": {"type": "string"},
                                "description": {"type": "string"},
                            },
                            "required": ["label"],
                        },
                    },
                },
                "required": ["question", "header", "multiSelect", "options"],
            },
        },
    },
    "required": ["questions"],
}

ASK_CHOICE_DESCRIPTION = (
    "Ask the user one or more structured multiple-choice questions, rendered as "
    "clickable options in the UI. Use this instead of asking in prose when you "
    "need the user to choose between options or clarify a decision. The user can "
    "always pick 'Other' and type a free-text answer. After calling this tool, "
    "end your turn and wait — the user's selection arrives as the next message."
)


async def ask_choice(args: dict[str, Any]) -> dict[str, Any]:
    # The question payload rides in the tool input, which is already streamed to
    # the UI. Nothing to do here but acknowledge so the model stops and waits.
    return ToolDescriptor.text_result(_ACK)


def build_interactive_descriptors() -> list[ToolDescriptor]:
    """Build the runtime-neutral interactive descriptors (just ``AskChoice``).

    The runner registers these only when ``AskChoice`` is in the Job's allowed
    tools, so ordinary read-only turns that don't need clarification never see it.
    """
    return [
        ToolDescriptor(
            name="AskChoice",
            description=ASK_CHOICE_DESCRIPTION,
            input_schema=ASK_CHOICE_SCHEMA,
            handler=ask_choice,
        )
    ]
