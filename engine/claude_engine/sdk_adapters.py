"""
Claude Agent SDK adapters for the shared engine_core capabilities.

``engine_core`` produces runtime-neutral ``ToolDescriptor``s (workspace,
AskChoice, mcp_router). The Claude runtime consumes capabilities as in-process
SDK MCP servers, so this module wraps a list of descriptors as a
``create_sdk_mcp_server`` and computes the ``mcp__<server>__<tool>`` allow-list
names the SDK requires.

It also maps resolved remote MCP servers into the SDK's http/sse server shape.
"""
from __future__ import annotations

from typing import Any

from claude_agent_sdk import create_sdk_mcp_server, tool

from engine_core.models.events import log
from engine_core.tools.descriptors import ToolDescriptor, invoke_safely


def _sdk_tool(descriptor: ToolDescriptor):
    """Wrap one ``ToolDescriptor`` as a Claude Agent SDK ``@tool`` function."""

    @tool(descriptor.name, descriptor.description, descriptor.input_schema)
    async def _fn(args: dict[str, Any]) -> dict[str, Any]:
        return await invoke_safely(descriptor, args)

    return _fn


def descriptors_to_server(server_name: str, descriptors: list[ToolDescriptor]):
    """Build an SDK MCP server exposing *descriptors* under *server_name*."""
    return create_sdk_mcp_server(
        name=server_name, version="0.1.0", tools=[_sdk_tool(d) for d in descriptors]
    )


def sdk_allowed_names(server_name: str, descriptors: list[ToolDescriptor]) -> list[str]:
    """Compute the ``mcp__<server>__<tool>`` allow-list names for *descriptors*."""
    return [f"mcp__{server_name}__{d.name}" for d in descriptors]


# ---------------------------------------------------------------------------
# Remote MCP servers → SDK http/sse shape
# ---------------------------------------------------------------------------

def build_remote_mcp(servers: list, reserved_names: set[str]) -> tuple[dict, list[str], bool]:
    """Map resolved remote MCP servers into SDK shape.

    Returns ``(mcp_servers, allow_names, want_defer)`` where ``allow_names`` are
    ``mcp__<name>`` prefix wildcards auto-approving each server's tools. Names
    colliding with ``reserved_names`` (e.g. the in-process servers) are skipped.
    """
    out: dict = {}
    allow: list[str] = []
    want_defer = False
    seen = set(reserved_names)
    for srv in servers:
        if srv.name in seen:
            log(f"Skipping remote MCP server with reserved/duplicate name {srv.name!r}")
            continue
        seen.add(srv.name)
        out[srv.name] = {"type": srv.type, "url": srv.url, "headers": srv.headers or {}}
        allow.append(f"mcp__{srv.name}")
        if srv.defer_loading:
            want_defer = True
    return out, allow, want_defer
