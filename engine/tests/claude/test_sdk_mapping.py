"""
Tests for core/sdk_mapping.py — SDK message stream → protocol events.

Uses real Claude Agent SDK dataclass types (StreamEvent, ResultMessage, etc.)
to construct test messages, so isinstance checks in sdk_mapping.py work
without a live API call.
"""
import json
import uuid
from collections.abc import AsyncIterator
from typing import Any

import pytest
from claude_agent_sdk.types import (
    AssistantMessage,
    ResultMessage,
    StreamEvent,
    SystemMessage,
    ToolResultBlock,
    UserMessage,
)

from claude_engine.core.sdk_mapping import process_sdk_messages
from engine_core.models.events import _BaseEvent

# ---------------------------------------------------------------------------
# Stub factories using real SDK types
# ---------------------------------------------------------------------------

def mk_stream_event(event: dict) -> StreamEvent:
    return StreamEvent(uuid=str(uuid.uuid4()), session_id="stub-session", event=event)


def mk_result(session_id: str = "sess-1", model: str = "claude-haiku-4-5") -> ResultMessage:
    return ResultMessage(
        subtype="success",
        duration_ms=100,
        duration_api_ms=90,
        is_error=False,
        num_turns=1,
        session_id=session_id,
    )


def mk_user_message(*blocks) -> UserMessage:
    return UserMessage(content=list(blocks))


def mk_tool_result_block(tool_use_id: str, text: str) -> ToolResultBlock:
    return ToolResultBlock(
        tool_use_id=tool_use_id,
        content=[{"type": "text", "text": text}],
    )


def mk_system() -> SystemMessage:
    return SystemMessage(subtype="status", data={})


# ---------------------------------------------------------------------------
# Async stream helper
# ---------------------------------------------------------------------------

async def _stream(*messages) -> AsyncIterator[Any]:
    for m in messages:
        yield m


async def run_stream(*messages) -> list[dict]:
    """Run process_sdk_messages over messages and return serialized events."""
    emitted: list[dict] = []

    def collect(ev: _BaseEvent) -> None:
        emitted.append(json.loads(ev.to_ndjson()))

    await process_sdk_messages(_stream(*messages), emit_fn=collect)
    return emitted


# ---------------------------------------------------------------------------
# Text streaming
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_text_block_streaming():
    events = await run_stream(
        mk_stream_event({"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}),
        mk_stream_event({"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "hello"}}),
        mk_stream_event({"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": " world"}}),
        mk_stream_event({"type": "content_block_stop", "index": 0}),
        mk_result(),
    )
    types = [e["type"] for e in events]
    assert types == ["text.start", "text.delta", "text.delta", "text.stop", "result"]
    assert events[1]["text"] == "hello"
    assert events[2]["text"] == " world"


# ---------------------------------------------------------------------------
# Thinking streaming
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_thinking_block_streaming():
    events = await run_stream(
        mk_stream_event({"type": "content_block_start", "index": 0, "content_block": {"type": "thinking", "thinking": ""}}),
        mk_stream_event({"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "Let me think"}}),
        mk_stream_event({"type": "content_block_stop", "index": 0}),
        mk_result(),
    )
    types = [e["type"] for e in events]
    assert types == ["thinking.start", "thinking.delta", "thinking.stop", "result"]
    assert events[1]["text"] == "Let me think"


# ---------------------------------------------------------------------------
# Tool streaming + agent.step ordering
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_tool_block_streaming_order():
    """agent.step must come AFTER tool.input_done (matching the original loop order)."""
    events = await run_stream(
        mk_stream_event({
            "type": "content_block_start", "index": 0,
            "content_block": {"type": "tool_use", "id": "tu_1", "name": "Read"},
        }),
        mk_stream_event({"type": "content_block_delta", "index": 0, "delta": {"type": "input_json_delta", "partial_json": '{"path"'}}),
        mk_stream_event({"type": "content_block_delta", "index": 0, "delta": {"type": "input_json_delta", "partial_json": ':"main.go"}'}}),
        mk_stream_event({"type": "content_block_stop", "index": 0}),
        mk_result(),
    )
    types = [e["type"] for e in events]
    assert types == [
        "tool.start",
        "tool.input_delta",
        "tool.input_delta",
        "tool.input_done",
        "agent.step",   # AFTER tool.input_done
        "result",
    ]
    assert events[0]["name"] == "Read"
    assert events[0]["id"] == "tu_1"
    assert events[-2]["tool"] == "Read"
    assert events[-2]["step"] == 1
    # tool.input_done carries the assembled name + input so consumers that don't
    # reassemble the streamed deltas (the Go orchestrator persisting AskChoice
    # questions) can read the final input directly.
    done = events[3]
    assert done["type"] == "tool.input_done"
    assert done["name"] == "Read"
    assert done["input"] == {"path": "main.go"}


@pytest.mark.asyncio
async def test_tool_input_done_omits_input_when_no_deltas():
    """A tool block with no streamed input emits input_done without an input key."""
    events = await run_stream(
        mk_stream_event({
            "type": "content_block_start", "index": 0,
            "content_block": {"type": "tool_use", "id": "tu_1", "name": "ListFiles"},
        }),
        mk_stream_event({"type": "content_block_stop", "index": 0}),
        mk_result(),
    )
    done = next(e for e in events if e["type"] == "tool.input_done")
    assert done["name"] == "ListFiles"
    assert "input" not in done  # exclude_none drops the unset input


# ---------------------------------------------------------------------------
# Tool result from UserMessage
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_tool_result_from_user_message():
    events = await run_stream(
        mk_user_message(mk_tool_result_block("tu_1", "file contents here")),
        mk_result(),
    )
    assert events[0]["type"] == "tool.result"
    assert events[0]["id"] == "tu_1"
    assert "file contents here" in events[0]["result"]


# ---------------------------------------------------------------------------
# ResultMessage: session_id and terminal event
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_result_event_carries_session_id():
    events = await run_stream(mk_result(session_id="sess-abc"))
    result = events[-1]
    assert result["type"] == "result"
    assert result["status"] == "success"
    assert result["sessionId"] == "sess-abc"


@pytest.mark.asyncio
async def test_process_returns_session_id():
    emitted = []
    session_id = await process_sdk_messages(
        _stream(mk_result(session_id="my-sid")),
        emit_fn=lambda ev: emitted.append(ev),
    )
    assert session_id == "my-sid"


@pytest.mark.asyncio
async def test_result_event_carries_usage_and_cost():
    # v8: normalized usage (incl. cache fields) + engine cost + turns ride in-band.
    result_msg = ResultMessage(
        subtype="success",
        duration_ms=100,
        duration_api_ms=90,
        is_error=False,
        num_turns=4,
        session_id="sess-u",
        total_cost_usd=0.0123,
        usage={
            "input_tokens": 100,
            "output_tokens": 50,
            "cache_read_input_tokens": 900,
            "cache_creation_input_tokens": 30,
        },
    )
    events = await run_stream(result_msg)
    result = events[-1]
    assert result["usage"] == {
        "input_tokens": 100, "output_tokens": 50,
        "cache_read_tokens": 900, "cache_creation_tokens": 30,
    }
    assert result["costUsd"] == 0.0123
    assert result["numTurns"] == 4


# ---------------------------------------------------------------------------
# Multiple blocks (interleaved thinking + text)
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_interleaved_thinking_and_text():
    events = await run_stream(
        mk_stream_event({"type": "content_block_start", "index": 0, "content_block": {"type": "thinking", "thinking": ""}}),
        mk_stream_event({"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "..."}}),
        mk_stream_event({"type": "content_block_stop", "index": 0}),
        mk_stream_event({"type": "content_block_start", "index": 1, "content_block": {"type": "text", "text": ""}}),
        mk_stream_event({"type": "content_block_delta", "index": 1, "delta": {"type": "text_delta", "text": "Answer"}}),
        mk_stream_event({"type": "content_block_stop", "index": 1}),
        mk_result(),
    )
    types = [e["type"] for e in events]
    assert types == [
        "thinking.start", "thinking.delta", "thinking.stop",
        "text.start", "text.delta", "text.stop",
        "result",
    ]


# ---------------------------------------------------------------------------
# Unknown / ignored StreamEvent types
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_unknown_stream_event_types_ignored():
    events = await run_stream(
        mk_stream_event({"type": "message_start", "message": {}}),
        mk_stream_event({"type": "message_delta", "delta": {}}),
        mk_stream_event({"type": "message_stop"}),
        mk_result(),
    )
    # Only the result event should be emitted.
    assert len(events) == 1
    assert events[0]["type"] == "result"


# ---------------------------------------------------------------------------
# SystemMessage is ignored
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_system_messages_ignored():
    events = await run_stream(mk_system(), mk_result())
    assert len(events) == 1
    assert events[0]["type"] == "result"


# ---------------------------------------------------------------------------
# AssistantMessage is ignored (streaming events cover content)
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_assistant_message_ignored():
    am = AssistantMessage(
        content=[],
        model="claude-haiku-4-5",
    )
    events = await run_stream(am, mk_result())
    assert len(events) == 1
    assert events[0]["type"] == "result"


# ---------------------------------------------------------------------------
# Step counter increments across multiple tool calls
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_multiple_tool_calls_increment_step():
    events = await run_stream(
        # Tool call 1
        mk_stream_event({"type": "content_block_start", "index": 0, "content_block": {"type": "tool_use", "id": "t1", "name": "Read"}}),
        mk_stream_event({"type": "content_block_stop", "index": 0}),
        # Tool call 2
        mk_stream_event({"type": "content_block_start", "index": 1, "content_block": {"type": "tool_use", "id": "t2", "name": "Grep"}}),
        mk_stream_event({"type": "content_block_stop", "index": 1}),
        mk_result(),
    )
    agent_steps = [e for e in events if e["type"] == "agent.step"]
    assert len(agent_steps) == 2
    assert agent_steps[0]["step"] == 1
    assert agent_steps[0]["tool"] == "Read"
    assert agent_steps[1]["step"] == 2
    assert agent_steps[1]["tool"] == "Grep"


# ---------------------------------------------------------------------------
# Sub-agent messages (parent_tool_use_id set) are filtered out
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_subagent_messages_filtered_out():
    """Sub-agent UserMessages and StreamEvents must be skipped.

    The SDK yields sub-agent UserMessages (tool results) but NOT their
    StreamEvents (content_block_start), so processing them would emit
    orphan tool.result events with no matching tool.start.
    """
    parent_tool_id = "toolu_parent"
    subagent_tool_id = "toolu_subagent"

    # Parent calls Agent tool
    parent_start = mk_stream_event({
        "type": "content_block_start", "index": 0,
        "content_block": {"type": "tool_use", "id": parent_tool_id, "name": "Agent"},
    })
    parent_stop = mk_stream_event({"type": "content_block_stop", "index": 0})

    # Sub-agent's UserMessage with tool result (has parent_tool_use_id)
    subagent_user = UserMessage(
        content=[mk_tool_result_block(subagent_tool_id, "ls output")],
        parent_tool_use_id=parent_tool_id,
    )

    # Sub-agent's StreamEvent (has parent_tool_use_id)
    subagent_stream = StreamEvent(
        uuid=str(uuid.uuid4()),
        session_id="stub-session",
        event={"type": "content_block_start", "index": 0,
               "content_block": {"type": "text", "id": "sub-text"}},
        parent_tool_use_id=parent_tool_id,
    )

    # Parent's UserMessage with Agent tool result (no parent_tool_use_id)
    parent_user = mk_user_message(mk_tool_result_block(parent_tool_id, "agent done"))

    events = await run_stream(
        parent_start, parent_stop,
        subagent_stream,    # should be skipped
        subagent_user,      # should be skipped
        parent_user,        # should produce tool.result for parent_tool_id
        mk_result(),
    )

    tool_results = [e for e in events if e["type"] == "tool.result"]
    assert len(tool_results) == 1
    assert tool_results[0]["id"] == parent_tool_id
    assert tool_results[0]["result"] == "agent done"

    # No text.start from the sub-agent's StreamEvent
    text_starts = [e for e in events if e["type"] == "text.start"]
    assert len(text_starts) == 0
