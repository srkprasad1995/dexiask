"""
Claude engine HTTP server.

A thin wrapper over the shared ``engine_core.create_app``: it injects the
``ClaudeRuntime`` and ``ClaudeSettings`` so the generic Agent Job Protocol server
(streaming NDJSON jobs) runs the Claude Agent SDK. The module-level ``app`` is the
uvicorn target.
"""
from __future__ import annotations

from engine_core import create_app as _create_app

from ..models.settings import ClaudeSettings, get_settings
from ..runtime import ClaudeRuntime


def create_app(settings: ClaudeSettings | None = None):
    """Build the FastAPI app wired to the Claude runtime."""
    return _create_app(ClaudeRuntime(), settings or get_settings())


# Module-level app instance (uvicorn target: ``claude_engine.server.app:app``).
app = create_app()
