"""
Typed protocol event models and NDJSON I/O.

Each event type is a Pydantic model whose ``type`` field is a Literal string
matching what ``web/src/lib/chat/go-sse-to-ui.ts`` expects.

DO NOT rename the ``type`` literals without updating:
  - web/src/lib/chat/go-sse-to-ui.ts
  - backend/internal/agent/protocol.go  (Event struct)
  - PROTOCOL.md
"""
from __future__ import annotations

import json
import logging
import sys
from typing import Annotated, Any, Literal

from pydantic import BaseModel, ConfigDict, Field

# Module logger. When observability is enabled, an OTLP handler is attached to
# the root logger and these records are shipped to Loki with the active span's
# trace context; otherwise they fall back to the stderr handler.
_logger = logging.getLogger("dexiask-engine")


# ---------------------------------------------------------------------------
# Base
# ---------------------------------------------------------------------------

class _BaseEvent(BaseModel):
    """Common config for all event models."""
    model_config = ConfigDict(populate_by_name=True, serialize_by_alias=True)

    def to_ndjson(self) -> str:
        """Serialize to a single NDJSON line (no trailing newline)."""
        return self.model_dump_json(exclude_none=True, by_alias=True)


# ---------------------------------------------------------------------------
# Text streaming
# ---------------------------------------------------------------------------

class TextStartEvent(_BaseEvent):
    type: Literal["text.start"] = "text.start"
    id: str


class TextDeltaEvent(_BaseEvent):
    type: Literal["text.delta"] = "text.delta"
    id: str
    text: str


class TextStopEvent(_BaseEvent):
    type: Literal["text.stop"] = "text.stop"
    id: str


# ---------------------------------------------------------------------------
# Thinking streaming
# ---------------------------------------------------------------------------

class ThinkingStartEvent(_BaseEvent):
    type: Literal["thinking.start"] = "thinking.start"
    id: str


class ThinkingDeltaEvent(_BaseEvent):
    type: Literal["thinking.delta"] = "thinking.delta"
    id: str
    text: str


class ThinkingStopEvent(_BaseEvent):
    type: Literal["thinking.stop"] = "thinking.stop"
    id: str


# ---------------------------------------------------------------------------
# Tool streaming
# ---------------------------------------------------------------------------

class ToolStartEvent(_BaseEvent):
    type: Literal["tool.start"] = "tool.start"
    id: str
    name: str


class ToolInputDeltaEvent(_BaseEvent):
    type: Literal["tool.input_delta"] = "tool.input_delta"
    id: str
    # Field name avoids shadowing BaseModel internals; alias preserves the wire name.
    partial_json: str = Field(..., alias="json", description="Partial JSON fragment of the tool input.")


class ToolInputDoneEvent(_BaseEvent):
    type: Literal["tool.input_done"] = "tool.input_done"
    id: str
    # Tool name and fully-assembled input, carried so consumers that don't
    # reassemble the streamed input deltas (the Go orchestrator persisting
    # interactive AskChoice questions) can read the final input directly.
    # Excluded from the wire when unset (to_ndjson uses exclude_none=True).
    name: str | None = None
    input: Any | None = None


class ToolResultEvent(_BaseEvent):
    type: Literal["tool.result"] = "tool.result"
    id: str
    result: str


# ---------------------------------------------------------------------------
# Agent progress
# ---------------------------------------------------------------------------

class AgentStepEvent(_BaseEvent):
    type: Literal["agent.step"] = "agent.step"
    step: int
    tool: str


# ---------------------------------------------------------------------------
# Terminal events
# ---------------------------------------------------------------------------

class Usage(BaseModel):
    """Normalized per-turn token usage (Protocol v8).

    The four canonical fields the engine maps its provider-specific usage onto.
    Wire names are snake_case and match the Go decoder
    (backend/internal/agent/protocol.go Usage). Absent counts are 0, never null.
    Built via ``engine_core.observability.normalize_usage``.
    """
    model_config = ConfigDict(populate_by_name=True, serialize_by_alias=True)

    input_tokens: int = 0
    output_tokens: int = 0
    cache_read_tokens: int = 0
    cache_creation_tokens: int = 0


class ResultEvent(_BaseEvent):
    type: Literal["result"] = "result"
    status: Literal["success"] = "success"
    model: str
    session_id: str | None = Field(
        default=None,
        alias="sessionId",
        description="SDK session ID for conversation resume.",
    )
    # Protocol v8: per-turn token usage + optional engine-reported cost, carried
    # in-band so the Go orchestrator can persist one usage_record per turn for
    # cost analytics. Dollars are authoritative from the backend pricing table;
    # ``cost_usd`` here is the engine's own figure, stored for validation only.
    usage: Usage | None = None
    cost_usd: float | None = Field(
        default=None,
        alias="costUsd",
        description="Engine-reported total cost in USD, if available (validation only).",
    )
    num_turns: int | None = Field(
        default=None,
        alias="numTurns",
        description="Number of agent loop turns, if the engine reports it.",
    )


class ErrorEvent(_BaseEvent):
    type: Literal["error"] = "error"
    message: str


# ---------------------------------------------------------------------------
# Discriminated union  (used by the HTTP server for typed decoding if needed)
# ---------------------------------------------------------------------------

Event = Annotated[
    TextStartEvent | TextDeltaEvent | TextStopEvent | ThinkingStartEvent | ThinkingDeltaEvent | ThinkingStopEvent | ToolStartEvent | ToolInputDeltaEvent | ToolInputDoneEvent | ToolResultEvent | AgentStepEvent | ResultEvent | ErrorEvent,
    Field(discriminator="type"),
]

# Ordered set of all event type strings — useful for tests.
ALL_EVENT_TYPES: frozenset[str] = frozenset({
    "text.start", "text.delta", "text.stop",
    "thinking.start", "thinking.delta", "thinking.stop",
    "tool.start", "tool.input_delta", "tool.input_done", "tool.result",
    "agent.step",
    "result",
    "error",
})


# ---------------------------------------------------------------------------
# I/O helpers
# ---------------------------------------------------------------------------

def emit(event: _BaseEvent) -> None:
    """Write one NDJSON event line to stdout and flush immediately."""
    sys.stdout.write(event.to_ndjson() + "\n")
    sys.stdout.flush()


def emit_raw(data: dict[str, Any]) -> None:
    """
    Write a raw dict as an NDJSON line.

    Use only in error-path code that runs before the typed models are
    available (e.g. the server startup guard for a missing API key).
    """
    sys.stdout.write(json.dumps(data) + "\n")
    sys.stdout.flush()


def log(msg: str) -> None:
    """Emit a diagnostic message (never reaches the client over NDJSON).

    Routed through stdlib logging so it reaches both stderr (local) and, when
    observability is enabled, the OTLP→Loki pipeline with trace correlation.
    """
    _logger.info(msg)
