"""Tests for the client-side deferred-MCP tool-search router (runtime-agnostic)."""
import json
from contextlib import asynccontextmanager

import pytest

from engine_core.capabilities.mcp_router import (
    RemoteMcpRouter,
    build_mcp_router_descriptors,
)
from engine_core.models.job import McpServerConfig
from engine_core.models.settings import BaseEngineSettings


class _Tool:
    def __init__(self, name, description, inputSchema):
        self.name = name
        self.description = description
        self.inputSchema = inputSchema


class _ListResult:
    def __init__(self, tools):
        self.tools = tools


class _Text:
    type = "text"

    def __init__(self, text):
        self.text = text


class _CallResult:
    def __init__(self, content, isError=False):
        self.content = content
        self.isError = isError


class FakeSession:
    def __init__(self, tools, counters):
        self._tools = tools
        self._c = counters

    async def list_tools(self):
        self._c["list"] += 1
        return _ListResult(self._tools)

    async def call_tool(self, name, arguments):
        self._c["call"] += 1
        return _CallResult([_Text(f"ran {name} {json.dumps(arguments)}")])


def make_router(tools_by_server, counters):
    servers = [
        McpServerConfig(name=n, type="http", url=f"https://{n}", defer_loading=True)
        for n in tools_by_server
    ]
    router = RemoteMcpRouter(servers, BaseEngineSettings())

    @asynccontextmanager
    async def fake_connect(name):
        yield FakeSession(tools_by_server[name], counters)

    router._connect = fake_connect  # type: ignore[assignment]
    return router


@pytest.mark.asyncio
async def test_search_ranks_and_returns_schema():
    tools = {
        "gh": [
            _Tool("create_issue", "Create a GitHub issue", {"type": "object", "properties": {"title": {}}}),
            _Tool("list_repos", "List repositories", {"type": "object", "properties": {}}),
        ]
    }
    r = make_router(tools, {"list": 0, "call": 0})
    res = await r.search("create issue")
    assert res[0]["name"] == "create_issue"
    assert res[0]["server"] == "gh"
    assert res[0]["input_schema"] == {"type": "object", "properties": {"title": {}}}


@pytest.mark.asyncio
async def test_search_caches_catalog():
    counters = {"list": 0, "call": 0}
    r = make_router({"gh": [_Tool("a", "alpha", {})]}, counters)
    await r.search("alpha")
    await r.search("alpha again")
    assert counters["list"] == 1


@pytest.mark.asyncio
async def test_search_unknown_server_raises():
    r = make_router({"gh": []}, {"list": 0, "call": 0})
    with pytest.raises(ValueError):
        await r.search("x", server="nope")


@pytest.mark.asyncio
async def test_proxy_call_ok():
    r = make_router({"gh": [_Tool("create_issue", "", {})]}, {"list": 0, "call": 0})
    res = await r.proxy_call("gh", "create_issue", {"title": "hi"})
    assert res["is_error"] is False
    assert "ran create_issue" in res["content"][0]["text"]


@pytest.mark.asyncio
async def test_proxy_call_unknown_server_is_error():
    r = make_router({"gh": []}, {"list": 0, "call": 0})
    res = await r.proxy_call("nope", "x", {})
    assert res["is_error"] is True


@pytest.mark.asyncio
async def test_proxy_call_remote_error_is_error():
    r = make_router({"gh": []}, {"list": 0, "call": 0})

    @asynccontextmanager
    async def boom(name):
        raise RuntimeError("conn refused")
        yield  # pragma: no cover

    r._connect = boom  # type: ignore[assignment]
    res = await r.proxy_call("gh", "x", {})
    assert res["is_error"] is True
    assert "conn refused" in res["content"][0]["text"]


def test_build_router_descriptors_names():
    r = make_router({"gh": []}, {"list": 0, "call": 0})
    descs = build_mcp_router_descriptors(r)
    assert [d.name for d in descs] == ["list_mcp_servers", "search_tools", "call_tool"]
