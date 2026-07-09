"""Tests for the local Ollama runtime — the compact no-API-key agent loop.

The Ollama HTTP boundary is stubbed (httpx.AsyncClient.stream) so the loop's
streaming, tool dispatch, session persistence, and event emission are exercised
without a server.
"""
import json

import pytest

from engine_core import local_runtime as lr_mod
from engine_core.models.events import (
    ResultEvent,
    TextDeltaEvent,
    ToolResultEvent,
    ToolStartEvent,
)
from engine_core.models.job import Job
from engine_core.models.settings import BaseEngineSettings
from engine_core.runner import run as core_run
from engine_core.runtime import AgentRuntime


class LocalSettings(BaseEngineSettings):
    """Settings with the local fallback configured."""

    @property
    def engine_api_key(self) -> str:
        return ""


class HostRuntime(AgentRuntime):
    """Minimal host runtime whose fallback is the real LocalOllamaRuntime."""

    def name(self) -> str:
        return "host"

    async def run(self, ctx):  # pragma: no cover - never used in these tests
        raise AssertionError("host runtime must not run")


def _stub_stream(monkeypatch, turns: list[list[dict]], captured: list[dict]):
    """Stub httpx.AsyncClient.stream to replay one chunk-list per model call."""

    calls = {"n": 0}

    class FakeResponse:
        def __init__(self, chunks):
            self._chunks = chunks

        def raise_for_status(self):
            pass

        async def aiter_lines(self):
            for c in self._chunks:
                yield json.dumps(c)

        async def __aenter__(self):
            return self

        async def __aexit__(self, *a):
            return False

    class FakeClient:
        def __init__(self, timeout=None):
            pass

        def stream(self, method, url, json=None):
            captured.append({"url": url, "payload": json})
            chunks = turns[min(calls["n"], len(turns) - 1)]
            calls["n"] += 1
            return FakeResponse(chunks)

        async def __aenter__(self):
            return self

        async def __aexit__(self, *a):
            return False

    monkeypatch.setattr(lr_mod.httpx, "AsyncClient", FakeClient)


def _job(tmp_path, **overrides) -> Job:
    return Job.model_validate({
        "messages": [{"role": "user", "content": "where is auth handled?"}],
        "role": "ask",
        "systemPrompt": "You answer questions about the codebase.",
        "workspacePath": str(tmp_path),
        "permissionMode": "dontAsk",
        "allowedTools": ["Read", "Grep"],
        **overrides,
    })


def _settings(tmp_path) -> LocalSettings:
    return LocalSettings(
        local_base_url="http://ollama:11434",
        local_model="qwen2.5:1.5b",
        session_store_path=str(tmp_path / "store"),
    )


@pytest.mark.asyncio
async def test_text_only_turn_streams_and_persists(monkeypatch, tmp_path):
    captured: list[dict] = []
    _stub_stream(monkeypatch, [[
        {"message": {"role": "assistant", "content": "In "}},
        {"message": {"role": "assistant", "content": "auth.go"}},
        {"done": True, "prompt_eval_count": 100, "eval_count": 5},
    ]], captured)

    events = []
    settings = _settings(tmp_path)
    sid = await core_run(_job(tmp_path), HostRuntime(), events.append, settings)

    assert sid is not None
    deltas = [e.text for e in events if isinstance(e, TextDeltaEvent)]
    assert "".join(deltas) == "In auth.go"
    result = next(e for e in events if isinstance(e, ResultEvent))
    assert result.model == "qwen2.5:1.5b"
    assert result.usage.input_tokens == 100 and result.usage.output_tokens == 5

    # The request went to the local server with the compact prompt (the Job's
    # system prompt — no skills addendum) and the workspace tools.
    payload = captured[0]["payload"]
    assert captured[0]["url"] == "http://ollama:11434/api/chat"
    assert payload["model"] == "qwen2.5:1.5b"
    assert payload["messages"][0] == {
        "role": "system", "content": "You answer questions about the codebase.",
    }
    assert {t["function"]["name"] for t in payload["tools"]} == {"Read", "Grep"}

    # History persisted for resume: next turn with the sessionId replays it.
    store_file = tmp_path / "store" / "local-sessions" / f"{sid}.json"
    history = json.loads(store_file.read_text())
    assert history[-1] == {"role": "assistant", "content": "In auth.go"}


@pytest.mark.asyncio
async def test_tool_call_loop_dispatches_builtin(monkeypatch, tmp_path):
    (tmp_path / "auth.go").write_text("package auth\n")
    captured: list[dict] = []
    _stub_stream(monkeypatch, [
        [  # turn 1: the model calls Read
            {"message": {"role": "assistant", "content": "", "tool_calls": [
                {"function": {"name": "Read", "arguments": {"path": str(tmp_path / "auth.go")}}},
            ]}},
            {"done": True, "prompt_eval_count": 10, "eval_count": 3},
        ],
        [  # turn 2: final answer
            {"message": {"role": "assistant", "content": "It is package auth."}},
            {"done": True, "prompt_eval_count": 4, "eval_count": 6},
        ],
    ], captured)

    events = []
    settings = _settings(tmp_path)
    sid = await core_run(_job(tmp_path), HostRuntime(), events.append, settings)

    assert sid is not None
    starts = [e for e in events if isinstance(e, ToolStartEvent)]
    assert [s.name for s in starts] == ["Read"]
    tool_result = next(e for e in events if isinstance(e, ToolResultEvent))
    assert "package auth" in tool_result.result

    # Second model call carries the tool exchange back.
    msgs = captured[1]["payload"]["messages"]
    assert msgs[-1]["role"] == "tool" and msgs[-1]["tool_name"] == "Read"
    assert msgs[-2]["role"] == "assistant" and msgs[-2]["tool_calls"]

    result = next(e for e in events if isinstance(e, ResultEvent))
    assert result.num_turns == 2
    assert result.usage.input_tokens == 14 and result.usage.output_tokens == 9


@pytest.mark.asyncio
async def test_unknown_tool_returns_error_result(monkeypatch, tmp_path):
    captured: list[dict] = []
    _stub_stream(monkeypatch, [
        [
            {"message": {"role": "assistant", "content": "", "tool_calls": [
                {"function": {"name": "bogus", "arguments": {}}},
            ]}},
            {"done": True},
        ],
        [
            {"message": {"role": "assistant", "content": "done"}},
            {"done": True},
        ],
    ], captured)

    events = []
    await core_run(_job(tmp_path), HostRuntime(), events.append, _settings(tmp_path))
    tool_result = next(e for e in events if isinstance(e, ToolResultEvent))
    assert "unknown tool" in tool_result.result


@pytest.mark.asyncio
async def test_session_resume_replays_history(monkeypatch, tmp_path):
    captured: list[dict] = []
    _stub_stream(monkeypatch, [[
        {"message": {"role": "assistant", "content": "Still auth.go."}},
        {"done": True},
    ]], captured)

    settings = _settings(tmp_path)
    store_dir = tmp_path / "store" / "local-sessions"
    store_dir.mkdir(parents=True)
    (store_dir / "sess-1.json").write_text(json.dumps([
        {"role": "user", "content": "where is auth handled?"},
        {"role": "assistant", "content": "In auth.go"},
    ]))

    events = []
    job = _job(tmp_path, sessionId="sess-1",
               messages=[{"role": "user", "content": "and sessions?"}])
    sid = await core_run(job, HostRuntime(), events.append, settings)
    assert sid == "sess-1"

    msgs = captured[0]["payload"]["messages"]
    # system + 2 history turns + the new user message.
    assert [m["role"] for m in msgs] == ["system", "user", "assistant", "user"]
    assert msgs[-1]["content"] == "and sessions?"

    history = json.loads((store_dir / "sess-1.json").read_text())
    assert len(history) == 4 and history[-1]["content"] == "Still auth.go."


@pytest.mark.asyncio
async def test_string_tool_arguments_are_parsed(monkeypatch, tmp_path):
    (tmp_path / "a.txt").write_text("hi\n")
    captured: list[dict] = []
    _stub_stream(monkeypatch, [
        [
            {"message": {"role": "assistant", "content": "", "tool_calls": [
                {"function": {"name": "Read",
                              "arguments": json.dumps({"path": str(tmp_path / "a.txt")})}},
            ]}},
            {"done": True},
        ],
        [
            {"message": {"role": "assistant", "content": "ok"}},
            {"done": True},
        ],
    ], captured)

    events = []
    await core_run(_job(tmp_path), HostRuntime(), events.append, _settings(tmp_path))
    tool_result = next(e for e in events if isinstance(e, ToolResultEvent))
    assert "hi" in tool_result.result


@pytest.mark.asyncio
async def test_oversized_tool_result_is_truncated(monkeypatch, tmp_path):
    big = tmp_path / "big.txt"
    big.write_text("x" * 50_000)
    _stub_stream(monkeypatch, [
        [
            {"message": {"role": "assistant", "content": "", "tool_calls": [
                {"function": {"name": "Read", "arguments": {"path": str(big)}}},
            ]}},
            {"done": True},
        ],
        [
            {"message": {"role": "assistant", "content": "ok"}},
            {"done": True},
        ],
    ], [])

    events = []
    await core_run(_job(tmp_path), HostRuntime(), events.append, _settings(tmp_path))
    tool_result = next(e for e in events if isinstance(e, ToolResultEvent))
    assert len(tool_result.result) <= lr_mod._MAX_TOOL_RESULT_CHARS + 100
    assert tool_result.result.endswith("[... truncated]")
