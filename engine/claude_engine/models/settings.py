"""
Claude engine configuration — ``BaseEngineSettings`` plus Anthropic credentials.

All the runtime-agnostic knobs (server, mount paths, tool limits, MCP router,
observability) live in ``engine_core.models.settings.BaseEngineSettings``. This
subclass adds the Anthropic API key/base URL and pins the Claude defaults and the
``CLAUDE_ENGINE_`` env prefix.

The default model is read from the bare ``CLAUDE_ENGINE_MODEL`` env var; the API
key from ``ANTHROPIC_API_KEY``. Both are captured here at construction, before
``engine_core.create_app`` scrubs the credential vars from the process env.

``Settings`` is an alias of ``ClaudeSettings`` for convenience.
"""
from __future__ import annotations

from functools import lru_cache

from pydantic import AliasChoices, Field
from pydantic_settings import SettingsConfigDict

from engine_core.models.settings import BaseEngineSettings


class ClaudeSettings(BaseEngineSettings):
    """Runtime knobs for the Claude engine, sourced from env vars."""

    model_config = SettingsConfigDict(
        env_prefix="CLAUDE_ENGINE_",
        case_sensitive=False,
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
        populate_by_name=True,
    )

    # ── Anthropic API ──────────────────────────────────────────────────────
    anthropic_api_key: str = Field(
        default="",
        alias="ANTHROPIC_API_KEY",
        description="Anthropic API key.  Required at runtime (unless supplied per-Job).",
    )
    anthropic_base_url: str = Field(
        default="",
        alias="ANTHROPIC_BASE_URL",
        description=(
            "Override API base URL (e.g. for a local Anthropic-compatible gateway). "
            "The Claude Agent SDK uses the Node CLI, which also honours this variable."
        ),
    )

    # ── Inference default (Claude) ─────────────────────────────────────────
    # Read from the bare CLAUDE_ENGINE_MODEL env var (explicit alias, so the
    # env_prefix is not applied a second time).
    default_model: str = Field(
        default="claude-sonnet-5",
        validation_alias=AliasChoices("CLAUDE_ENGINE_MODEL", "CLAUDE_ENGINE_DEFAULT_MODEL"),
        description="Model used when the Job does not specify one.",
    )

    # ── Provider credential plumbing (consumed by engine_core.runner) ──────
    @property
    def engine_api_key(self) -> str:
        return self.anthropic_api_key

    @property
    def engine_base_url(self) -> str:
        return self.anthropic_base_url


# Convenience alias.
Settings = ClaudeSettings


@lru_cache(maxsize=1)
def get_settings() -> ClaudeSettings:
    """Return the singleton settings instance (clear the cache in tests)."""
    return ClaudeSettings()
