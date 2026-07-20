"""Tests for the runtime-agnostic runner: credential resolution (Job key vs env
fallback), prompt composition, attachment force-enable, and remote-MCP split —
exercised through a FakeRuntime that captures the composed RunContext."""
import pytest

from engine_core.models.events import ErrorEvent
from engine_core.models.job import Job
from engine_core.models.settings import BaseEngineSettings
from engine_core.runner import run
from engine_core.runtime import AgentRuntime, RunContext


class FakeSettings(BaseEngineSettings):
    """Engine settings with a configurable env provider key/url fallback."""

    fake_key: str = ""
    fake_url: str = ""

    @property
    def engine_api_key(self) -> str:
        return self.fake_key

    @property
    def engine_base_url(self) -> str:
        return self.fake_url


class FakeRuntime(AgentRuntime):
    def __init__(self):
        self.ctx: RunContext | None = None

    def name(self) -> str:
        return "fake"

    def credential_env_names(self) -> tuple[str, str]:
        return ("FAKE_API_KEY", "FAKE_BASE_URL")

    async def run(self, ctx: RunContext) -> str | None:
        self.ctx = ctx
        return "sess-fake"


def _collect():
    events = []
    return events, events.append


@pytest.mark.asyncio
async def test_missing_key_emits_error(tmp_path):
    rt = FakeRuntime()
    events, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
    })
    sid = await run(job, rt, emit, FakeSettings())
    assert sid is None
    assert any(isinstance(e, ErrorEvent) and "FAKE_API_KEY" in e.message for e in events)


@pytest.mark.asyncio
async def test_job_credentials_take_precedence(tmp_path):
    rt = FakeRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "apiKey": "job-key",
        "baseUrl": "https://job",
    })
    # Job creds win over the env fallback.
    sid = await run(job, rt, emit, FakeSettings(fake_key="env-key", fake_url="https://env"))
    assert sid == "sess-fake"
    assert rt.ctx.provider_env == {"FAKE_API_KEY": "job-key", "FAKE_BASE_URL": "https://job"}


@pytest.mark.asyncio
async def test_env_credential_fallback(tmp_path):
    # The Job carries no api_key → the engine falls back to its own env-configured
    # key (the OSS single-user path). The run proceeds.
    rt = FakeRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
    })
    sid = await run(job, rt, emit, FakeSettings(fake_key="env-key", fake_url="https://env"))
    assert sid == "sess-fake"
    assert rt.ctx.provider_env == {"FAKE_API_KEY": "env-key", "FAKE_BASE_URL": "https://env"}


@pytest.mark.asyncio
async def test_env_base_url_optional(tmp_path):
    # A blank base URL (env or Job) means "use the provider default": not sent.
    rt = FakeRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
    })
    await run(job, rt, emit, FakeSettings(fake_key="env-key"))
    assert rt.ctx.provider_env == {"FAKE_API_KEY": "env-key"}


@pytest.mark.asyncio
async def test_system_prompt_includes_base(tmp_path):
    rt = FakeRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "systemPrompt": "BASE-PROMPT",
        "skillsPath": str(tmp_path / "noskills"),
        "apiKey": "k",
    })
    await run(job, rt, emit, FakeSettings())
    assert rt.ctx.system_prompt.startswith("BASE-PROMPT")


@pytest.mark.asyncio
async def test_remote_mcp_split_native_vs_deferred(tmp_path):
    rt = FakeRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "mcpServers": [
            {"name": "hot", "type": "http", "url": "https://hot"},
            {"name": "cold", "type": "http", "url": "https://cold", "deferLoading": True},
        ],
        "apiKey": "k",
    })
    await run(job, rt, emit, FakeSettings())
    assert [s.name for s in rt.ctx.remote_native] == ["hot"]
    assert [s.name for s in rt.ctx.remote_deferred] == ["cold"]
    # Deferred servers get a router descriptor set + a system-prompt addendum.
    assert [d.name for d in rt.ctx.builtin_tools.mcp_router] == [
        "list_mcp_servers", "search_tools", "call_tool",
    ]
    assert "cold" in rt.ctx.system_prompt


@pytest.mark.asyncio
async def test_ask_choice_gated_on_allowed_tools(tmp_path):
    rt = FakeRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "allowedTools": ["Read", "AskChoice"],
        "apiKey": "k",
    })
    await run(job, rt, emit, FakeSettings())
    assert [d.name for d in rt.ctx.builtin_tools.interactive] == ["AskChoice"]
    assert [d.name for d in rt.ctx.builtin_tools.workspace] == ["Read"]


@pytest.mark.asyncio
async def test_attachment_force_enables_read(tmp_path):
    (tmp_path / "doc.txt").write_text("hello")
    rt = FakeRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{
            "role": "user", "content": "see file",
            "attachments": [{"kind": "file", "path": str(tmp_path / "doc.txt"),
                             "mediaType": "text/plain", "filename": "doc.txt"}],
        }],
        "workspacePath": str(tmp_path),
        "permissionMode": "default",
        "allowedTools": [],
        "apiKey": "k",
    })
    await run(job, rt, emit, FakeSettings())
    # Read/Glob/Grep force-enabled; default mode downgraded to dontAsk so the
    # workspace descriptors are built.
    assert "Read" in rt.ctx.requested_tools
    assert rt.ctx.permission_mode == "dontAsk"
    assert [d.name for d in rt.ctx.builtin_tools.workspace] == ["Read", "Glob", "Grep"]


# --- Local model fallback (no API key → swap to the local runtime) ---


class FakeLocalRuntime(FakeRuntime):
    """Capturing stand-in for the local-fallback runtime."""

    def name(self) -> str:
        return "fake-local"

    def credential_env_names(self) -> tuple[str, str]:
        return ("LOCAL_API_KEY", "LOCAL_BASE_URL")

    async def run(self, ctx: RunContext) -> str | None:
        self.ctx = ctx
        return "sess-local"


class SwappingRuntime(FakeRuntime):
    """FakeRuntime whose local fallback is a capturing FakeLocalRuntime."""

    def __init__(self):
        super().__init__()
        self.local = FakeLocalRuntime()

    def local_runtime(self) -> FakeLocalRuntime:
        return self.local


@pytest.mark.asyncio
async def test_local_fallback_swaps_runtime(tmp_path):
    # No Job key, no env key, but a local server is configured → the run is
    # dispatched to the runtime's local_runtime() with the local model.
    rt = SwappingRuntime()
    events, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "model": "claude-sonnet-5",
    })
    settings = FakeSettings(local_base_url="http://ollama:11434", local_model="qwen2.5:1.5b")
    sid = await run(job, rt, emit, settings)
    assert sid == "sess-local"
    assert not any(isinstance(e, ErrorEvent) for e in events)
    assert rt.ctx is None  # the original runtime never ran
    assert rt.local.ctx.model == "qwen2.5:1.5b"
    assert rt.local.ctx.provider_env == {
        "LOCAL_API_KEY": "local",
        "LOCAL_BASE_URL": "http://ollama:11434",
    }


@pytest.mark.asyncio
async def test_local_fallback_without_local_model_keeps_job_model(tmp_path):
    rt = SwappingRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "model": "some-model",
    })
    await run(job, rt, emit, FakeSettings(local_base_url="http://ollama:11434"))
    assert rt.local.ctx.model == "some-model"


@pytest.mark.asyncio
async def test_local_runtime_drops_interactive_tools(tmp_path):
    # The default local runtime doesn't support interactive tools, so AskChoice
    # must not be registered even when the Job allows it.
    class NonInteractiveLocal(FakeLocalRuntime):
        def supports_interactive(self) -> bool:
            return False

    rt = SwappingRuntime()
    rt.local = NonInteractiveLocal()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "allowedTools": ["Read", "AskChoice"],
        "permissionMode": "dontAsk",
    })
    await run(job, rt, emit, FakeSettings(local_base_url="http://ollama:11434"))
    assert rt.local.ctx.builtin_tools.interactive == []


@pytest.mark.asyncio
async def test_job_key_wins_over_local_fallback(tmp_path):
    rt = SwappingRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "model": "claude-sonnet-5",
        "apiKey": "job-key",
    })
    settings = FakeSettings(local_base_url="http://ollama:11434", local_model="qwen2.5:1.5b")
    await run(job, rt, emit, settings)
    assert rt.local.ctx is None  # local runtime not used
    assert rt.ctx.model == "claude-sonnet-5"
    assert rt.ctx.provider_env == {"FAKE_API_KEY": "job-key"}


@pytest.mark.asyncio
async def test_env_key_wins_over_local_fallback(tmp_path):
    rt = SwappingRuntime()
    _, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
        "model": "claude-sonnet-5",
    })
    settings = FakeSettings(
        fake_key="env-key",
        local_base_url="http://ollama:11434", local_model="qwen2.5:1.5b",
    )
    await run(job, rt, emit, settings)
    assert rt.local.ctx is None
    assert rt.ctx.model == "claude-sonnet-5"
    assert rt.ctx.provider_env == {"FAKE_API_KEY": "env-key"}


@pytest.mark.asyncio
async def test_no_key_and_no_local_still_errors(tmp_path):
    rt = FakeRuntime()
    events, emit = _collect()
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "workspacePath": str(tmp_path),
    })
    sid = await run(job, rt, emit, FakeSettings())
    assert sid is None
    assert any(isinstance(e, ErrorEvent) and "COMPOSE_PROFILES=local" in e.message for e in events)
