"""Tests for sdk_adapters.py — descriptor → SDK MCP server + remote MCP mapping."""
from claude_engine.sdk_adapters import (
    build_remote_mcp,
    descriptors_to_server,
    sdk_allowed_names,
)
from engine_core.capabilities.workspace import build_workspace_descriptors
from engine_core.runtime import RemoteMcp


def test_sdk_allowed_names_prefixes_server_and_tool(tmp_path):
    descs = build_workspace_descriptors(str(tmp_path), ["Read", "Glob"])
    names = sdk_allowed_names("workspace", descs)
    assert names == ["mcp__workspace__Read", "mcp__workspace__Glob"]


def test_descriptors_to_server_builds_named_server(tmp_path):
    descs = build_workspace_descriptors(str(tmp_path), ["Read"])
    server = descriptors_to_server("workspace", descs)
    # create_sdk_mcp_server returns an SDK config dict/object; it must be truthy
    # and constructed without raising.
    assert server is not None


def test_build_remote_mcp_maps_shape_and_allow_names():
    servers = [
        RemoteMcp(name="indexer", type="http", url="https://idx", headers={"A": "b"}),
        RemoteMcp(name="slack", type="sse", url="https://slack", defer_loading=True),
    ]
    out, allow, want_defer = build_remote_mcp(servers, reserved_names=set())
    assert out["indexer"] == {"type": "http", "url": "https://idx", "headers": {"A": "b"}}
    assert out["slack"]["type"] == "sse"
    assert set(allow) == {"mcp__indexer", "mcp__slack"}
    assert want_defer is True


def test_build_remote_mcp_skips_reserved_names():
    servers = [RemoteMcp(name="workspace", type="http", url="https://x")]
    out, allow, _ = build_remote_mcp(servers, reserved_names={"workspace"})
    assert out == {}
    assert allow == []
