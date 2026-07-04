"""
Map Claude Agent SDK message stream to Agent Job Protocol NDJSON events.

The SDK yields these message types when ``include_partial_messages=True``:
  - ``SystemMessage``       — init/status signals; ignored.
  - ``StreamEvent``         — raw Anthropic API streaming events (plain dict in
                              ``.event``).  Contains ``content_block_start``,
                              ``content_block_delta``, ``content_block_stop``.
  - ``AssistantMessage``    — fully assembled assistant turn (blocks); ignored
                              here since streaming events cover the deltas.
  - ``UserMessage``         — tool results returned by the SDK after execution.
  - ``ResultMessage``       — terminal event carrying ``session_id`` and cost.

The protocol event names (``text.start``, ``tool.input_delta``, …) are
identical to what the old hand-rolled ``streaming.py`` emitted; the Go SSE
handler and ``go-sse-to-ui.ts`` are unchanged.

Key difference vs the old ``streaming.py``:
  ``StreamEvent.event`` is a plain ``dict`` (not a typed SDK object), so we
  use ``.get("type")`` instead of ``getattr(event, "type")``.

Emit ordering (matching the original hand-rolled loop):
  1. ``tool.start``        — content_block_start for tool_use block
  2. ``tool.input_delta``  — content_block_delta with input_json_delta
  3. ``tool.input_done``   — content_block_stop for tool_use block
  4. ``agent.step``        — emitted immediately after tool.input_done
  5. ``tool.result``       — UserMessage ToolResultBlock (SDK executed the tool)
  6. ``result``            — ResultMessage (final, carries session_id)
"""
from __future__ import annotations

import json
from collections.abc import AsyncIterator, Callable
from typing import Any

from claude_agent_sdk.types import (
    ResultMessage,
    StreamEvent,
    ToolResultBlock,
    UserMessage,
)
from opentelemetry import trace

from engine_core import observability as obs
from engine_core.models.events import (
    AgentStepEvent,
    ResultEvent,
    TextDeltaEvent,
    TextStartEvent,
    TextStopEvent,
    ThinkingDeltaEvent,
    ThinkingStartEvent,
    ThinkingStopEvent,
    ToolInputDeltaEvent,
    ToolInputDoneEvent,
    ToolResultEvent,
    ToolStartEvent,
    _BaseEvent,
    log,
)

# Block type tags stored by index.
_TEXT = "text"
_THINKING = "thinking"
_TOOL = "tool_use"


async def process_sdk_messages(
    messages: AsyncIterator[Any],
    emit_fn: Callable[[_BaseEvent], None],
    fallback_model: str = "",
) -> str | None:
    """
    Consume the Claude Agent SDK message iterator and emit protocol events.

    Args:
        messages:  Async iterator from ``query()`` or ``ClaudeSDKClient``.
        emit_fn:   Callback that receives a typed event model.

    Returns:
        The SDK ``session_id`` from the terminal ``ResultMessage``, or
        ``None`` if the stream ended without one (e.g. error path).
    """
    # index → (block_id, block_type, tool_name | "")
    open_blocks: dict[int, tuple[str, str, str]] = {}
    # index → accumulated input_json_delta fragments for an open tool block.
    tool_input_buffers: dict[int, str] = {}
    step = 0
    session_id: str | None = None

    async for msg in messages:
        # Sub-agent messages (spawned by the Agent tool) carry a non-None
        # parent_tool_use_id.  The SDK yields their UserMessages (tool
        # results) but NOT their StreamEvents (content_block_start etc.),
        # so processing them would emit orphan tool.result events with no
        # matching tool.start — causing "No tool invocation found" in the
        # AI SDK on the frontend.  Skip them entirely; the parent-level
        # tool.result for the Agent tool will arrive separately.
        if getattr(msg, "parent_tool_use_id", None) is not None:
            continue

        # ── StreamEvent: raw Anthropic API events (dicts) ─────────────────
        if isinstance(msg, StreamEvent):
            ev: dict[str, Any] = msg.event if isinstance(msg.event, dict) else {}
            etype = ev.get("type")

            if etype == "content_block_start":
                block = ev.get("content_block", {})
                idx: int = ev.get("index", 0)
                block_id = f"b{idx + 1}"
                btype = block.get("type", "")

                if btype == _TEXT:
                    open_blocks[idx] = (block_id, _TEXT, "")
                    emit_fn(TextStartEvent(id=block_id))

                elif btype == _THINKING:
                    open_blocks[idx] = (block_id, _THINKING, "")
                    emit_fn(ThinkingStartEvent(id=block_id))

                elif btype == _TOOL:
                    # Prefer the SDK-assigned tool_use id for correlation.
                    tool_id = block.get("id") or block_id
                    tool_name = block.get("name", "")
                    open_blocks[idx] = (tool_id, _TOOL, tool_name)
                    emit_fn(ToolStartEvent(id=tool_id, name=tool_name))

            elif etype == "content_block_delta":
                idx = ev.get("index", 0)
                block_id, block_type, tool_name = open_blocks.get(
                    idx, (f"b{idx + 1}", _TEXT, "")
                )
                delta = ev.get("delta", {})
                dtype = delta.get("type", "")

                if dtype == "text_delta":
                    emit_fn(TextDeltaEvent(id=block_id, text=delta.get("text", "")))

                elif dtype == "thinking_delta":
                    emit_fn(ThinkingDeltaEvent(id=block_id, text=delta.get("thinking", "")))

                elif dtype == "input_json_delta":
                    fragment = delta.get("partial_json", "")
                    tool_input_buffers[idx] = tool_input_buffers.get(idx, "") + fragment
                    emit_fn(
                        ToolInputDeltaEvent(
                            id=block_id,
                            partial_json=fragment,
                        )
                    )
                # signature_delta (for thinking blocks) is intentionally ignored.

            elif etype == "content_block_stop":
                idx = ev.get("index", 0)
                block_id, block_type, tool_name = open_blocks.pop(
                    idx, (f"b{idx + 1}", _TEXT, "")
                )

                if block_type == _TEXT:
                    emit_fn(TextStopEvent(id=block_id))

                elif block_type == _THINKING:
                    emit_fn(ThinkingStopEvent(id=block_id))

                elif block_type == _TOOL:
                    # Parse the assembled input so consumers that don't
                    # reassemble the streamed deltas (the Go orchestrator
                    # persisting AskChoice questions) can read it directly.
                    raw_input = tool_input_buffers.pop(idx, "")
                    tool_input: Any = None
                    if raw_input:
                        try:
                            tool_input = json.loads(raw_input)
                        except json.JSONDecodeError:
                            tool_input = None
                    emit_fn(
                        ToolInputDoneEvent(
                            id=block_id, name=tool_name, input=tool_input
                        )
                    )
                    # agent.step comes immediately after tool.input_done,
                    # matching the order of the original hand-rolled loop.
                    step += 1
                    emit_fn(AgentStepEvent(step=step, tool=tool_name))
                    log(f"Tool step #{step}: {tool_name} (id={block_id})")

            # All other raw event types (message_start, message_delta,
            # message_stop) are intentionally ignored.

        # ── UserMessage: tool results returned by the SDK ──────────────────
        elif isinstance(msg, UserMessage):
            for block in msg.content:
                # ToolResultBlock has .tool_use_id and .content but no .type field.
                if isinstance(block, ToolResultBlock):
                    result_str = _extract_tool_result_text(block.content)
                    emit_fn(ToolResultEvent(id=block.tool_use_id, result=result_str))

        # ── ResultMessage: terminal event ──────────────────────────────────
        elif isinstance(msg, ResultMessage):
            session_id = getattr(msg, "session_id", None)
            # Some backends (e.g. LM Studio) omit model on the result; fall back
            # to the job's resolved model so metrics aren't labeled "unknown".
            model_id = (getattr(msg, "model", "") or "") or fallback_model
            usage = getattr(msg, "usage", None)
            cost = getattr(msg, "total_cost_usd", None)
            num_turns = getattr(msg, "num_turns", None)
            log(
                f"Done. session_id={session_id!r} turns={num_turns} cost=${cost}"
            )
            # Token/cost/turn metrics → OTLP (ops dashboards only; no protocol change).
            obs.record_result(model_id, usage, cost, num_turns)
            span = trace.get_current_span()
            span.set_attribute("gen_ai.request.model", model_id)
            in_tok = obs._usage_get(usage, "input_tokens")
            out_tok = obs._usage_get(usage, "output_tokens")
            if in_tok:
                span.set_attribute("gen_ai.usage.input_tokens", in_tok)
            if out_tok:
                span.set_attribute("gen_ai.usage.output_tokens", out_tok)
            if num_turns is not None:
                span.set_attribute("dexiask.num_turns", num_turns)
            # v8: also carry normalized usage + engine cost in-band for the
            # orchestrator's per-turn cost record. Claude's SDK usage exposes
            # cache_read_input_tokens / cache_creation_input_tokens.
            emit_fn(ResultEvent(
                model=model_id,
                session_id=session_id,
                usage=obs.normalize_usage(usage),
                cost_usd=float(cost) if cost is not None else None,
                num_turns=int(num_turns) if num_turns is not None else None,
            ))

        # AssistantMessage is ignored — all content arrives via StreamEvent deltas.

    return session_id


def _extract_tool_result_text(content: Any) -> str:
    """
    Extract a plain string from the heterogeneous content returned by the
    Agent SDK in a ``ToolResultBlock``.

    The content can be:
      - A string (rare legacy path).
      - A list of ``{"type": "text", "text": "..."}`` dicts.
      - A list of typed block objects with ``.type == "text"`` and ``.text``.
      - Something else — fall back to ``str()``.
    """
    if isinstance(content, str):
        return content

    if isinstance(content, list):
        parts: list[str] = []
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                parts.append(item.get("text", ""))
            elif hasattr(item, "type") and getattr(item, "type") == "text":
                parts.append(getattr(item, "text", ""))
            else:
                parts.append(str(item))
        return "\n".join(parts)

    return str(content)
