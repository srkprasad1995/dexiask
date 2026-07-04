"""Tests for capabilities/workspace.py — WorkspaceTools path-jail + descriptors."""
import pytest

from engine_core.capabilities.workspace import (
    WorkspaceTools,
    build_workspace_descriptors,
)
from engine_core.models.settings import BaseEngineSettings


@pytest.fixture
def ws(tmp_path) -> WorkspaceTools:
    s = BaseEngineSettings(max_read_lines=100, max_glob_matches=50, max_grep_matches=50)
    return WorkspaceTools(str(tmp_path), settings=s)


@pytest.fixture
def sample_workspace(tmp_path):
    (tmp_path / "main.go").write_text("package main\n\nfunc main() {}\n")
    (tmp_path / "util.go").write_text("package main\n\nfunc helper() string { return \"ok\" }\n")
    sub = tmp_path / "pkg"
    sub.mkdir()
    (sub / "types.go").write_text("package pkg\n\ntype Foo struct{}\n")
    return tmp_path


def test_traversal_rejected(ws):
    assert "Access denied" in ws.read({"path": "../../etc/passwd"})


def test_absolute_path_rerooted(tmp_path):
    (tmp_path / "hello.txt").write_text("hi")
    w = WorkspaceTools(str(tmp_path), settings=BaseEngineSettings())
    assert "hi" in w.read({"path": "/hello.txt"})


def test_absolute_path_inside_jail_used_directly(tmp_path):
    # Attachment path-refs give the FULL container path (already under the
    # workspace root). It must be read as-is, not re-rooted (which would double
    # the prefix and 'not find' the file).
    sub = tmp_path / "attachments"
    sub.mkdir()
    (sub / "secret.txt").write_text("SAPPHIRE-99")
    w = WorkspaceTools(str(tmp_path), settings=BaseEngineSettings())
    assert "SAPPHIRE-99" in w.read({"path": str(sub / "secret.txt")})


def test_directory_rejected(ws, tmp_path):
    (tmp_path / "subdir").mkdir()
    assert "directory" in ws.read({"path": "subdir"}).lower()


def test_missing_file_rejected(ws):
    assert "does not exist" in ws.read({"path": "nonexistent.go"})


def test_read_basic(tmp_path):
    (tmp_path / "file.txt").write_text("line1\nline2\nline3\n")
    w = WorkspaceTools(str(tmp_path), settings=BaseEngineSettings())
    result = w.read({"path": "file.txt"})
    assert "line1" in result
    assert "     1\t" in result


def test_read_with_offset_and_limit(tmp_path):
    (tmp_path / "f.txt").write_text("\n".join(str(i) for i in range(1, 21)))
    w = WorkspaceTools(str(tmp_path), settings=BaseEngineSettings(max_read_lines=100))
    result = w.read({"path": "f.txt", "offset": 5, "limit": 3})
    assert "5" in result and "7" in result and "8" not in result


def test_read_limit_from_settings(tmp_path):
    (tmp_path / "big.txt").write_text("\n".join("x" for _ in range(200)))
    w = WorkspaceTools(str(tmp_path), settings=BaseEngineSettings(max_read_lines=10))
    assert w.read({"path": "big.txt"}).count("\n") <= 11


def test_glob_finds_files(sample_workspace):
    w = WorkspaceTools(str(sample_workspace), settings=BaseEngineSettings())
    result = w.glob({"pattern": "*.go"})
    assert "main.go" in result and "util.go" in result


def test_glob_no_match(ws):
    assert "No files matched" in ws.glob({"pattern": "*.nonexistent"})


def test_glob_truncation(tmp_path):
    for i in range(10):
        (tmp_path / f"file{i}.go").write_text("x")
    w = WorkspaceTools(str(tmp_path), settings=BaseEngineSettings(max_glob_matches=5))
    assert "truncated" in w.glob({"pattern": "*.go"})


def test_grep_finds_pattern(sample_workspace):
    w = WorkspaceTools(str(sample_workspace), settings=BaseEngineSettings())
    result = w.grep({"pattern": "func"})
    assert "main.go" in result and "func" in result


def test_grep_no_match(sample_workspace):
    w = WorkspaceTools(str(sample_workspace), settings=BaseEngineSettings())
    assert "No matches" in w.grep({"pattern": "XYZNOTFOUND"})


def test_grep_invalid_regex(ws):
    result = ws.grep({"pattern": "[invalid"})
    assert "invalid regex" in result.lower() or "Error" in result


def test_grep_traversal_rejected(ws):
    assert "Access denied" in ws.grep({"pattern": "x", "path": "../../etc"})


def test_grep_glob_filter(sample_workspace):
    w = WorkspaceTools(str(sample_workspace), settings=BaseEngineSettings())
    result = w.grep({"pattern": "package", "glob": "types.go"})
    assert "types.go" in result and "main.go" not in result


# ── Descriptors ────────────────────────────────────────────────────────────

def test_build_workspace_descriptors_subset(tmp_path):
    descs = build_workspace_descriptors(str(tmp_path), ["Read"])
    assert [d.name for d in descs] == ["Read"]


def test_build_workspace_descriptors_all(tmp_path):
    descs = build_workspace_descriptors(str(tmp_path), ["Read", "Glob", "Grep"])
    assert [d.name for d in descs] == ["Read", "Glob", "Grep"]


def test_build_workspace_descriptors_empty(tmp_path):
    assert build_workspace_descriptors(str(tmp_path), []) == []


@pytest.mark.asyncio
async def test_descriptor_handler_returns_content(tmp_path):
    (tmp_path / "a.txt").write_text("hello")
    descs = build_workspace_descriptors(str(tmp_path), ["Read"])
    out = await descs[0].handler({"path": "a.txt"})
    assert out["content"][0]["text"].endswith("hello") or "hello" in out["content"][0]["text"]

