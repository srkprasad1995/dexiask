"""
Runtime-agnostic engine configuration.

``BaseEngineSettings`` carries every knob that is *not* tied to a specific model
provider — the HTTP server, container mount paths, tool limits, the deferred-MCP
router, the session store, and observability. Each engine subclasses it to add
its own provider credentials (``ANTHROPIC_API_KEY`` for Claude) and set its
own ``env_prefix`` and ``default_model``.

Provider credentials come from the Job (``job.api_key`` / ``job.base_url``)
when the orchestrator supplies them, else from the engine env via the
``engine_api_key`` / ``engine_base_url`` accessors. When neither yields a key,
the runner falls back to the local model server (``local_base_url`` /
``local_model`` — the docker-compose Ollama sidecar) if one is configured.

Use a per-engine cached accessor (``get_settings``) so tests can monkeypatch the
singleton; ``default_settings()`` here returns a bare base instance for the
capability handlers that only need the tool limits.
"""
from __future__ import annotations

from functools import lru_cache

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class BaseEngineSettings(BaseSettings):
    """Provider-agnostic runtime knobs shared by every engine."""

    model_config = SettingsConfigDict(
        env_prefix="ENGINE_",
        case_sensitive=False,
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
        populate_by_name=True,
    )

    # ── Inference defaults (subclasses override default_model) ──────────────
    default_model: str = Field(
        default="",
        description="Model used when the Job does not specify one.",
    )
    max_turns: int = Field(
        default=20,
        ge=1,
        le=200,
        description="Maximum agent loop iterations.",
    )
    max_tokens: int = Field(
        default=8192,
        ge=256,
        le=65536,
        description="Maximum output tokens per model call.",
    )

    # ── Generation-knob defaults (used when the Job sets none) ──────────────
    # The orchestrator resolves role override → workspace default → platform
    # default and sends concrete maxTokens/maxTurns on every Job, so these
    # env-level defaults mainly back-stop effort/thinking, which the Job sends
    # only when explicitly configured. Empty/false = provider default.
    default_effort: str = Field(
        default="",
        description="Effort level (low|medium|high|xhigh|max) when the Job sets none.",
    )
    default_thinking: bool = Field(
        default=False,
        description="Enable extended thinking by default when the Job sets none.",
    )
    default_thinking_display: str = Field(
        default="summarized",
        description="Thinking display (summarized|omitted) when thinking is on.",
    )

    # ── HTTP server ────────────────────────────────────────────────────────
    server_host: str = Field(default="0.0.0.0", description="Bind host for the HTTP server.")
    server_port: int = Field(default=8080, ge=1, le=65535, description="Bind port.")

    # ── Container mount paths (defaults match the Dockerfile VOLUME) ───────
    default_workspace_path: str = Field(default="/workspace")
    default_skills_path: str = Field(default="/skills")

    # ── Workspace tool limits ──────────────────────────────────────────────
    max_read_lines: int = Field(
        default=2000,
        ge=1,
        description="Default maximum lines returned by the Read tool.",
    )
    max_glob_matches: int = Field(
        default=500,
        ge=1,
        description="Maximum file paths returned by the Glob tool.",
    )
    max_grep_matches: int = Field(
        default=500,
        ge=1,
        description="Maximum match lines returned by the Grep tool.",
    )

    # ── Session store ──────────────────────────────────────────────────────
    session_store_path: str = Field(
        default="/root/.claude",
        description=(
            "Directory where the runtime stores session/transcript data. "
            "Mount a volume here so sessions survive container restarts."
        ),
    )

    # ── Deferred MCP (client-side tool-search router) ──────────────────────
    mcp_search_max_results: int = Field(
        default=5,
        ge=1,
        le=50,
        description="Max tools returned by the mcp_router search_tools tool.",
    )
    mcp_tool_call_timeout_s: float = Field(
        default=30.0,
        gt=0,
        description="Timeout (seconds) for a single deferred-MCP tools/list or tools/call.",
    )

    # ── Observability (OpenTelemetry) ──────────────────────────────────────
    otel_exporter_otlp_endpoint: str = Field(
        default="",
        alias="OTEL_EXPORTER_OTLP_ENDPOINT",
        description=(
            "Base OTLP/HTTP endpoint of the OTLP Collector "
            "(e.g. http://host.docker.internal:4318). Empty disables telemetry."
        ),
    )
    otel_sdk_disabled: bool = Field(
        default=False,
        alias="OTEL_SDK_DISABLED",
        description="Hard kill-switch for all OpenTelemetry export.",
    )
    otel_service_name: str = Field(
        default="dexiask-engine",
        description="service.name resource attribute on exported telemetry.",
    )
    otel_content_sample_rate: float = Field(
        default=0.05,
        ge=0.0,
        le=1.0,
        description=(
            "Probability of attaching full prompt content to a span "
            "(errors are always captured)."
        ),
    )

    # ── Local model fallback (no-API-key mode) ─────────────────────────────
    # When neither the Job nor the env yields an API key, the runner runs jobs
    # against this provider-compatible local server (the docker-compose Ollama
    # sidecar) instead of erroring. Empty base URL = fallback unavailable.
    local_base_url: str = Field(
        default="",
        description=(
            "Base URL of a local provider-compatible model server "
            "(e.g. http://ollama:11434). Used only when no API key is configured."
        ),
    )
    local_model: str = Field(
        default="",
        description="Model served locally; replaces the Job's model on fallback.",
    )
    local_remote_tools: str = Field(
        default="*",
        description=(
            "Comma-separated allowlist of remote MCP tool names the local "
            "runtime exposes to the model, or '*' for every tool the attached "
            "servers offer. Narrow it (e.g. 'semantic_search,get_chunk') to "
            "shrink the prompt when a small model gets confused by many tools."
        ),
    )
    local_request_timeout_s: float = Field(
        default=300.0,
        gt=0,
        description="Per-request timeout (seconds) for local model calls.",
    )

    # ── Provider credentials (subclasses override) ─────────────────────────
    # Env-configured credentials, used when the Job carries none (the OSS
    # single-user path).
    @property
    def engine_api_key(self) -> str:
        """Provider API key, if any is configured in env."""
        return ""

    @property
    def engine_base_url(self) -> str:
        """Provider base URL, if any is configured in env."""
        return ""


@lru_cache(maxsize=1)
def default_settings() -> BaseEngineSettings:
    """Cached bare base instance, for capability handlers that only read limits."""
    return BaseEngineSettings()
