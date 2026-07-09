"""
The ``AgentRuntime`` abstraction — the single thing the engine implements.

The shared ``runner`` composes everything runtime-agnostic (system prompt with
skills, the built-in capabilities, attachment ingestion, credential resolution,
remote-MCP split) into a ``RunContext`` and hands it to a concrete
``AgentRuntime``. The runtime's only job is to drive its own agent loop and
translate its streaming output into protocol events via ``ctx.emit``.

The Claude engine is the one implementation of this ABC.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Callable
from dataclasses import dataclass, field

from .models.events import _BaseEvent
from .models.job import Job
from .models.settings import BaseEngineSettings
from .tools.descriptors import ToolDescriptor


@dataclass
class RemoteMcp:
    """A resolved remote MCP server (subset of the Job's McpServerConfig)."""

    name: str
    type: str
    url: str
    description: str = ""
    headers: dict[str, str] = field(default_factory=dict)
    defer_loading: bool = False


@dataclass
class BuiltinTools:
    """The shared capabilities the core wants exposed to the model, as
    runtime-neutral descriptors. The runtime decides HOW to surface them
    (for Claude: in-process SDK MCP servers)."""

    workspace: list[ToolDescriptor] = field(default_factory=list)
    interactive: list[ToolDescriptor] = field(default_factory=list)
    mcp_router: list[ToolDescriptor] = field(default_factory=list)

    def all_descriptors(self) -> list[ToolDescriptor]:
        return [
            *self.workspace,
            *self.interactive,
            *self.mcp_router,
        ]


@dataclass
class RunContext:
    """Everything the shared core composes and hands to the runtime for one turn."""

    job: Job
    system_prompt: str
    model: str
    workspace_path: str
    session_id: str | None
    # Per-conversation session-store directory (Job.session_store_path).
    # None → the runtime keeps its SDK default location.
    session_store_path: str | None
    builtin_tools: BuiltinTools
    remote_native: list[RemoteMcp]
    remote_deferred: list[RemoteMcp]
    provider_env: dict[str, str]
    permission_mode: str
    # Effective tool whitelist after attachment force-enable. The runtime reads
    # this for provider built-ins (e.g. WebSearch/WebFetch) not covered by
    # BuiltinTools.
    requested_tools: list[str]
    settings: BaseEngineSettings
    emit: Callable[[_BaseEvent], None]

    @property
    def messages(self) -> list:
        return self.job.messages

    @property
    def has_attachments(self) -> bool:
        return any(m.attachments for m in self.job.messages)

    # ── Generation-knob resolution (Job override → engine Settings default) ──
    # Centralised here so the runtime resolves the knobs identically.
    @property
    def max_tokens(self) -> int:
        return self.job.max_tokens if self.job.max_tokens is not None else self.settings.max_tokens

    @property
    def max_turns(self) -> int:
        return self.job.max_turns if self.job.max_turns is not None else self.settings.max_turns

    @property
    def effort(self) -> str:
        """Effort level, or "" when neither the Job nor Settings set one."""
        return self.job.effort or self.settings.default_effort

    @property
    def thinking_enabled(self) -> bool:
        if self.job.thinking is not None:
            return True
        return self.settings.default_thinking

    @property
    def thinking_display(self) -> str:
        if self.job.thinking is not None:
            return self.job.thinking.display
        return self.settings.default_thinking_display

    @property
    def fallback_model(self) -> str:
        return self.job.fallback_model or ""

    @property
    def betas(self) -> list[str]:
        return list(self.job.betas)


class AgentRuntime(ABC):
    """One agent runtime (the Claude Agent SDK)."""

    @abstractmethod
    def name(self) -> str:
        """Canonical runtime id."""

    @abstractmethod
    async def run(self, ctx: RunContext) -> str | None:
        """Execute one turn. Stream events via ``ctx.emit``. Return the session
        id to persist for resume, or ``None`` on error (after emitting an
        ``ErrorEvent``)."""

    # ── Credential plumbing ────────────────────────────────────────────────
    def credential_env_names(self) -> tuple[str, str]:
        """The (api_key, base_url) environment variable names this runtime's
        provider expects. The shared runner uses these to build
        ``ctx.provider_env`` from the resolved credentials."""
        return ("ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL")

    def local_runtime(self) -> AgentRuntime:
        """The runtime used for the local-model fallback (no API key; jobs run
        against ``settings.local_base_url``). Defaults to the built-in compact
        Ollama loop — heavyweight CLI-based runtimes are impractical against a
        small local model (their scaffolding alone is tens of thousands of
        prompt tokens)."""
        from .local_runtime import LocalOllamaRuntime

        return LocalOllamaRuntime()

    # ── Capability flags (let the shared core degrade gracefully) ──────────
    def supports_thinking(self) -> bool:
        return False

    def supports_session_resume(self) -> bool:
        return True

    def supports_interactive(self) -> bool:
        return True
