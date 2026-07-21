"""
Tests for ClaudeRuntime wiring, exercised through the shared runner.

These run ``engine_core.runner.run(job, ClaudeRuntime(), ...)`` with the SDK
boundary patched (ClaudeAgentOptions + query + process_sdk_messages), and assert
on the ClaudeAgentOptions kwargs the runtime builds: credential env mapping
(Job key vs env fallback), native vs deferred MCP attachment, generation knobs,
and the AskChoice shadowing.
"""
import pytest

from claude_engine import runtime as rt_mod
from claude_engine.models.settings import Settings
from claude_engine.runtime import ClaudeRuntime
from engine_core.models.job import Job
from engine_core.runner import run as core_run


def _patch_sdk(monkeypatch):
    captured: dict = {}

    def fake_options(**kwargs):
        captured.update(kwargs)
        return object()

    async def fake_process(messages, emit_fn, fallback_model=""):
        return "sess-x"

    monkeypatch.setattr(rt_mod, "ClaudeAgentOptions", fake_options)
    monkeypatch.setattr(rt_mod, "query", lambda prompt, options: None)
    monkeypatch.setattr(rt_mod, "process_sdk_messages", fake_process)
    return captured


def _settings(key="engine-key", base=""):
    s = Settings(_env_file=None)  # type: ignore[call-arg]
    s.anthropic_api_key = key
    s.anthropic_base_url = base
    return s


def _job(tmp_path, **overrides) -> Job:
    return Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "role": "ask",
        "skillsPath": str(tmp_path),
        "workspacePath": str(tmp_path),
        "apiKey": "job-key",
        **overrides,
    })


@pytest.mark.asyncio
async def test_job_credentials_take_precedence(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    job = _job(tmp_path, apiKey="ui-key", baseUrl="https://ui.example")
    await core_run(job, ClaudeRuntime(), lambda e: None, _settings(base="https://engine.example"))
    assert captured["env"]["ANTHROPIC_API_KEY"] == "ui-key"
    assert captured["env"]["ANTHROPIC_BASE_URL"] == "https://ui.example"


@pytest.mark.asyncio
async def test_env_key_fallback_when_job_has_none(monkeypatch, tmp_path):
    # Job carries no api_key → the engine falls back to its env-configured key
    # (the OSS single-user path). Run reaches the SDK boundary.
    captured = _patch_sdk(monkeypatch)
    job = _job(tmp_path, apiKey="")
    result = await core_run(
        job, ClaudeRuntime(), lambda e: None,
        _settings(key="engine-key", base="https://engine.example"),
    )
    assert result == "sess-x"
    assert captured["env"]["ANTHROPIC_API_KEY"] == "engine-key"
    assert captured["env"]["ANTHROPIC_BASE_URL"] == "https://engine.example"


@pytest.mark.asyncio
async def test_missing_key_everywhere_emits_actionable_error(monkeypatch, tmp_path):
    _patch_sdk(monkeypatch)
    events = []
    result = await core_run(_job(tmp_path, apiKey=""), ClaudeRuntime(), events.append, _settings(key=""))
    assert result is None
    assert any("ANTHROPIC_API_KEY" in getattr(ev, "message", "") for ev in events)


@pytest.mark.asyncio
async def test_blank_base_url_is_omitted(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    job = _job(tmp_path, apiKey="ui-key")
    await core_run(job, ClaudeRuntime(), lambda e: None, _settings(key="ignored", base=""))
    assert captured["env"]["ANTHROPIC_API_KEY"] == "ui-key"
    assert "ANTHROPIC_BASE_URL" not in captured["env"]


@pytest.mark.asyncio
async def test_max_output_tokens_from_settings(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    await core_run(_job(tmp_path), ClaudeRuntime(), lambda e: None, _settings())
    assert captured["env"]["CLAUDE_CODE_MAX_OUTPUT_TOKENS"] == str(_settings().max_tokens)


@pytest.mark.asyncio
async def test_generation_knobs_from_job(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    job = _job(
        tmp_path,
        maxTokens=16384,
        maxTurns=50,
        effort="xhigh",
        thinking={"type": "adaptive", "display": "omitted"},
        fallbackModel="claude-sonnet-5",
        betas=["context-1m-2025-08-07"],
    )
    await core_run(job, ClaudeRuntime(), lambda e: None, _settings())
    assert captured["env"]["CLAUDE_CODE_MAX_OUTPUT_TOKENS"] == "16384"
    assert captured["max_turns"] == 50
    assert captured["effort"] == "xhigh"
    assert captured["thinking"] == {"type": "adaptive", "display": "omitted"}
    assert captured["fallback_model"] == "claude-sonnet-5"
    assert captured["betas"] == ["context-1m-2025-08-07"]


@pytest.mark.asyncio
async def test_generation_knobs_default_to_settings(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    s = _settings()
    s.default_effort = "high"
    s.default_thinking = True
    await core_run(_job(tmp_path), ClaudeRuntime(), lambda e: None, s)
    assert captured["env"]["CLAUDE_CODE_MAX_OUTPUT_TOKENS"] == str(s.max_tokens)
    assert captured["max_turns"] == s.max_turns
    assert captured["effort"] == "high"
    assert captured["thinking"] == {"type": "adaptive", "display": "summarized"}
    assert "fallback_model" not in captured
    assert "betas" not in captured


@pytest.mark.asyncio
async def test_session_resume_and_store(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    store = tmp_path / "session"
    job = _job(tmp_path, sessionId="sess-prev", sessionStorePath=str(store))
    await core_run(job, ClaudeRuntime(), lambda e: None, _settings())
    assert captured["resume"] == "sess-prev"
    assert captured["env"]["CLAUDE_CONFIG_DIR"] == str(store)
    assert store.exists()


@pytest.mark.asyncio
async def test_read_only_web_tools_allowlisted(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    job = _job(tmp_path, allowedTools=["Read", "Glob", "Grep", "WebSearch", "WebFetch"])
    await core_run(job, ClaudeRuntime(), lambda e: None, _settings())
    assert "mcp__workspace__Read" in captured["allowed_tools"]
    assert "WebSearch" in captured["allowed_tools"]
    assert "WebFetch" in captured["allowed_tools"]


@pytest.mark.asyncio
async def test_wires_deferred_via_router(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    job = _job(tmp_path, mcpServers=[
        {"name": "hot", "type": "http", "url": "https://hot"},
        {"name": "cold", "type": "http", "url": "https://cold", "deferLoading": True},
    ])
    sid = await core_run(job, ClaudeRuntime(), lambda e: None, _settings())
    assert sid == "sess-x"
    servers = captured["mcp_servers"]
    assert "hot" in servers              # native attached directly
    assert "mcp_router" in servers       # deferred routed
    assert "cold" not in servers         # deferred NOT attached
    assert "mcp__hot" in captured["allowed_tools"]
    assert "mcp__mcp_router__search_tools" in captured["allowed_tools"]
    assert not any(a.startswith("mcp__cold") for a in captured["allowed_tools"])


@pytest.mark.asyncio
async def test_askchoice_shadows_builtin(monkeypatch, tmp_path):
    captured = _patch_sdk(monkeypatch)
    job = _job(tmp_path, allowedTools=["AskChoice"], permissionMode="dontAsk")
    await core_run(job, ClaudeRuntime(), lambda e: None, _settings())
    assert "interactive" in captured["mcp_servers"]
    assert "mcp__interactive__AskChoice" in captured["allowed_tools"]
    assert "AskUserQuestion" in captured.get("disallowed_tools", [])
