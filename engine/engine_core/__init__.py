"""
Dexiask engine core — the runtime-agnostic shared library for the engine.

The Claude engine implements ``AgentRuntime`` and hands it to ``create_app``; the
rest — Agent Job Protocol HTTP server, NDJSON events, skills, observability, and
the built-in workspace / AskChoice / mcp_router capabilities — is shared from here.
"""
__version__ = "1.0.0"

from .app import create_app
from .runtime import AgentRuntime, BuiltinTools, RemoteMcp, RunContext

__all__ = ["create_app", "AgentRuntime", "BuiltinTools", "RemoteMcp", "RunContext"]
