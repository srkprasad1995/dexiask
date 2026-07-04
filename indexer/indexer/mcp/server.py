"""
Streamable-HTTP MCP server.

Builds an ``mcp.server.lowlevel.Server`` from the tool registry and exposes it
over streamable HTTP so an MCP client (the agent engine's ``mcp_router``) can
connect via ``streamablehttp_client``.
"""
from __future__ import annotations

from contextlib import asynccontextmanager

from ..context import IndexerContext
from .tools import build_tools, dispatch


def build_mcp_server(ctx: IndexerContext):
    """Construct the low-level MCP server wired to the tool registry."""
    from mcp import types
    from mcp.server.lowlevel import Server

    server: Server = Server("dexiask-indexer")
    tools = build_tools()

    @server.list_tools()
    async def _list_tools() -> list:
        return [
            types.Tool(name=t.name, description=t.description, inputSchema=t.input_schema)
            for t in tools
        ]

    @server.call_tool()
    async def _call_tool(name: str, arguments: dict) -> list:
        text = await dispatch(ctx, name, arguments or {})
        return [types.TextContent(type="text", text=text)]

    return server


def build_session_manager(ctx: IndexerContext):
    """Wrap the MCP server in a StreamableHTTPSessionManager (stateless JSON)."""
    from mcp.server.streamable_http_manager import StreamableHTTPSessionManager

    server = build_mcp_server(ctx)
    return StreamableHTTPSessionManager(app=server, json_response=True, stateless=True)


@asynccontextmanager
async def mcp_lifespan(manager):
    """Run the session manager's task group for the app's lifetime."""
    async with manager.run():
        yield
