"""Tests for models/events.py — NDJSON wire shapes and the event union."""
import json

from engine_core.models.events import (
    ALL_EVENT_TYPES,
    AgentStepEvent,
    ErrorEvent,
    ResultEvent,
    TextDeltaEvent,
    ToolInputDeltaEvent,
    ToolInputDoneEvent,
    Usage,
)


def test_text_delta_ndjson():
    ev = TextDeltaEvent(id="b1", text="hello")
    assert json.loads(ev.to_ndjson()) == {"type": "text.delta", "id": "b1", "text": "hello"}


def test_tool_input_delta_uses_json_wire_key():
    ev = ToolInputDeltaEvent(partial_json='{"path"', id="t1")
    wire = json.loads(ev.to_ndjson())
    # The wire key is "json", not "partial_json".
    assert wire["json"] == '{"path"'
    assert "partial_json" not in wire


def test_tool_input_done_omits_unset_input():
    ev = ToolInputDoneEvent(id="t1", name="Read")
    wire = json.loads(ev.to_ndjson())
    assert wire["name"] == "Read"
    assert "input" not in wire  # exclude_none drops the unset input


def test_agent_step_shape():
    wire = json.loads(AgentStepEvent(step=2, tool="Grep").to_ndjson())
    assert wire == {"type": "agent.step", "step": 2, "tool": "Grep"}


def test_result_event_carries_usage_and_cost():
    ev = ResultEvent(
        model="claude-sonnet-5",
        session_id="sess-1",
        usage=Usage(input_tokens=100, output_tokens=50, cache_read_tokens=900),
        cost_usd=0.0123,
        num_turns=4,
    )
    wire = json.loads(ev.to_ndjson())
    assert wire["type"] == "result"
    assert wire["status"] == "success"
    assert wire["sessionId"] == "sess-1"
    assert wire["usage"] == {
        "input_tokens": 100, "output_tokens": 50,
        "cache_read_tokens": 900, "cache_creation_tokens": 0,
    }
    assert wire["costUsd"] == 0.0123
    assert wire["numTurns"] == 4


def test_error_event_shape():
    assert json.loads(ErrorEvent(message="boom").to_ndjson()) == {"type": "error", "message": "boom"}


def test_all_event_types_are_ask_scoped():
    """The trimmed event set carries no project/execute event types."""
    assert "result" in ALL_EVENT_TYPES
    assert "text.delta" in ALL_EVENT_TYPES
    assert "project.stage_transition" not in ALL_EVENT_TYPES
    assert "execute.phase" not in ALL_EVENT_TYPES
    assert "task.pr_opened" not in ALL_EVENT_TYPES
