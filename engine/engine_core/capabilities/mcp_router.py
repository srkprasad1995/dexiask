"""
Client-side Tool Search proxy for deferred MCP servers.

Agent runtimes typically load every attached MCP server's full tool definitions
into context up front, with no native per-tool deferral. To make the
``deferLoading`` flag real, servers marked deferred are NOT attached. Instead
this module exposes ONE small set of tools the agent uses to discover and invoke
the deferred servers' tools on demand:

  - ``list_mcp_servers()``           — what deferred servers exist
  - ``search_tools(query, server?)`` — find tools by capability (+ their schema)
  - ``call_tool(server, name, args)``— proxy-execute a discovered tool

The engine acts as the MCP *client* to the deferred servers (via the ``mcp``
package), fetching ``tools/list`` lazily and caching it for the run. Connections
are opened per invocation and closed immediately, so only catalog *data* is
retained — no long-lived sessions to leak. The router is fully runtime-agnostic;
``build_mcp_router_descriptors`` wraps it as ``ToolDescriptor``s.
"""
from __future__ import annotations

import asyncio
import json
import re
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from mcp.client.session import ClientSession
from mcp.client.sse import sse_client
from mcp.client.streamable_http import streamablehttp_client

from .. import observability as obs
from ..models.events import log
from ..models.settings import BaseEngineSettings
from ..tools.descriptors import ToolDescriptor

_WORD = re.compile(r"[a-z0-9]+")

MCP_ROUTER_TOOL_NAMES = ("list_mcp_servers", "search_tools", "call_tool")


def _tokenize(text: str) -> set[str]:
    return set(_WORD.findall((text or "").lower()))


class RemoteMcpRouter:
    """Lazily discovers and proxies tools for a set of deferred MCP servers."""

    def __init__(self, deferred_servers: list, settings: BaseEngineSettings) -> None:
        # name -> {transport, url, headers, description}
        self._registry: dict[str, dict[str, Any]] = {
            s.name: {
                "transport": s.type,
                "url": s.url,
                "headers": s.headers or {},
                "description": s.description or "",
            }
            for s in deferred_servers
        }
        # name -> [{name, description, input_schema}], filled on first search.
        self._catalog: dict[str, list[dict[str, Any]]] = {}
        self._max_results = settings.mcp_search_max_results
        self._timeout = settings.mcp_tool_call_timeout_s

    @property
    def server_names(self) -> list[str]:
        return list(self._registry)

    def list_servers(self) -> list[dict[str, str]]:
        return [
            {"name": name, "description": cfg["description"], "transport": cfg["transport"]}
            for name, cfg in self._registry.items()
        ]

    @asynccontextmanager
    async def _connect(self, name: str) -> AsyncIterator[ClientSession]:
        """Open a short-lived, initialized MCP client session to *name*."""
        cfg = self._registry[name]
        headers = cfg["headers"] or None
        if cfg["transport"] == "sse":
            async with sse_client(cfg["url"], headers=headers) as (read, write):
                async with ClientSession(read, write) as session:
                    await session.initialize()
                    yield session
        else:  # "http" (streamable HTTP)
            async with streamablehttp_client(cfg["url"], headers=headers) as (read, write, _get_sid):
                async with ClientSession(read, write) as session:
                    await session.initialize()
                    yield session

    async def _ensure_catalog(self, name: str) -> list[dict[str, Any]]:
        """Fetch + cache the tool list for *name* (once per run)."""
        if name in self._catalog:
            return self._catalog[name]
        async with self._connect(name) as session:
            result = await asyncio.wait_for(session.list_tools(), timeout=self._timeout)
        tools = [
            {
                "name": t.name,
                "description": t.description or "",
                "input_schema": t.inputSchema or {"type": "object"},
            }
            for t in result.tools
        ]
        self._catalog[name] = tools
        log(f"mcp_router: fetched {len(tools)} tools from {name!r}")
        return tools

    @staticmethod
    def _score(query_tokens: set[str], tool: dict[str, Any]) -> float:
        name = tool["name"].lower()
        name_tokens = _tokenize(tool["name"])
        desc_tokens = _tokenize(tool["description"])
        arg_tokens: set[str] = set()
        for k in (tool.get("input_schema") or {}).get("properties", {}):
            arg_tokens |= _tokenize(k)
        score = (
            3.0 * len(query_tokens & name_tokens)
            + 1.5 * len(query_tokens & arg_tokens)
            + 1.0 * len(query_tokens & desc_tokens)
        )
        if any(tok in name for tok in query_tokens):
            score += 1.0
        return score

    async def search(self, query: str, server: str | None = None) -> list[dict[str, Any]]:
        if server is not None and server not in self._registry:
            raise ValueError(f"unknown server {server!r}; available: {self.server_names}")
        names = [server] if server else self.server_names
        q = _tokenize(query)
        scored: list[tuple[float, str, dict[str, Any]]] = []
        for name in names:
            try:
                for t in await self._ensure_catalog(name):
                    scored.append((self._score(q, t), name, t))
            except Exception as e:  # one bad server must not sink the search
                log(f"mcp_router: catalog fetch failed for {name!r}: {e}")
        # Highest score first; ties keep discovery order. Always return something.
        scored.sort(key=lambda x: x[0], reverse=True)
        return [
            {
                "server": name,
                "name": t["name"],
                "description": t["description"],
                "input_schema": t["input_schema"],
            }
            for _, name, t in scored[: self._max_results]
        ]

    @staticmethod
    def _convert_result(result: Any) -> dict[str, Any]:
        blocks: list[dict[str, Any]] = []
        for c in getattr(result, "content", []) or []:
            ctype = getattr(c, "type", None)
            if ctype == "text":
                blocks.append({"type": "text", "text": c.text})
            elif ctype == "image":
                blocks.append({"type": "image", "data": c.data, "mimeType": c.mimeType})
            else:
                blocks.append({"type": "text", "text": str(getattr(c, "text", c))})
        if not blocks:
            blocks = [{"type": "text", "text": "(no content)"}]
        return {"content": blocks, "is_error": bool(getattr(result, "isError", False))}

    async def proxy_call(self, server: str, name: str, arguments: dict[str, Any] | None) -> dict[str, Any]:
        if server not in self._registry:
            return {
                "content": [{"type": "text", "text": f"unknown server {server!r}; available: {self.server_names}"}],
                "is_error": True,
            }
        try:
            async with self._connect(server) as session:
                result = await asyncio.wait_for(
                    session.call_tool(name, arguments or {}), timeout=self._timeout
                )
            return self._convert_result(result)
        except Exception as e:
            return {"content": [{"type": "text", "text": f"call_tool {server}/{name} failed: {e}"}], "is_error": True}


def build_mcp_router_descriptors(router: RemoteMcpRouter) -> list[ToolDescriptor]:
    """Wrap a ``RemoteMcpRouter`` as runtime-neutral descriptors."""

    async def list_mcp_servers(_args: dict[str, Any]) -> dict[str, Any]:
        with obs.tool_span("mcp_list_servers"):
            return ToolDescriptor.text_result(json.dumps(router.list_servers()))

    async def search_tools(args: dict[str, Any]) -> dict[str, Any]:
        with obs.tool_span("mcp_search", **{"mcp.query": args.get("query")}) as span:
            matches = await router.search(args["query"], args.get("server"))
            span.set_attribute("mcp.matches", len(matches))
            return ToolDescriptor.text_result(json.dumps(matches, indent=2))

    async def call_tool(args: dict[str, Any]) -> dict[str, Any]:
        server, name = args.get("server", ""), args.get("name", "")
        with obs.tool_span("mcp_call", **{"mcp.server": server, "mcp.tool": name}):
            return await router.proxy_call(server, name, args.get("arguments") or {})

    return [
        ToolDescriptor(
            "list_mcp_servers",
            "List the additional MCP servers whose tools are available on demand "
            "(not preloaded). Use search_tools to find their tools.",
            {"type": "object", "properties": {}},
            list_mcp_servers,
        ),
        ToolDescriptor(
            "search_tools",
            "Search the deferred MCP servers for tools matching a capability. "
            "Returns matches with their server, name, description and input_schema. "
            "Call this before call_tool to discover what is available.",
            {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Capability to find, e.g. 'create github issue'"},
                    "server": {"type": "string", "description": "Optional: restrict to one server name"},
                },
                "required": ["query"],
            },
            search_tools,
        ),
        ToolDescriptor(
            "call_tool",
            "Invoke a tool discovered via search_tools. Pass the server and name from "
            "the search result and arguments matching that tool's input_schema.",
            {
                "type": "object",
                "properties": {
                    "server": {"type": "string"},
                    "name": {"type": "string"},
                    "arguments": {"type": "object", "description": "Arguments matching the tool's input_schema"},
                },
                "required": ["server", "name"],
            },
            call_tool,
        ),
    ]
