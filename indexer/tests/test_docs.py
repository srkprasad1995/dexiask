import json
from pathlib import Path

import pytest

from indexer.config import IndexerConfig, RepoConfig
from indexer.context import IndexerContext
from indexer.docs import build_skeleton, load_skeleton, save_skeleton
from indexer.docs.skeleton import _extract_symbols
from indexer.git.mirror import Mirror
from indexer.mcp.tools import dispatch
from indexer.settings import Settings


@pytest.fixture
def git_and_settings(sample_repo: Path, tmp_path: Path):
    mirror = Mirror(tmp_path / "m" / "r.git", sample_repo, branch="main")
    mirror.ensure()
    return mirror.repo(), Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))


def test_extract_symbols_languages():
    assert ("func", "Start", 1) in _extract_symbols("func Start() {}", "go")
    assert ("type", "Job", 1) in _extract_symbols("type Job struct{}", "go")
    ts = _extract_symbols("export class Foo {}\nexport function bar() {}", "typescript")
    assert ("class", "Foo", 1) in ts
    assert _extract_symbols("whatever", "unknownlang") == []


def test_build_skeleton_extracts_symbols(git_and_settings):
    git, settings = git_and_settings
    skel = build_skeleton(settings, "r", git, "main")
    assert skel["commit"]
    assert skel["file_count"] == 3
    names = {s["name"] for s in skel["symbols"]}
    assert {"alpha", "beta", "gamma"} <= names
    assert all("refs" in s for s in skel["top_symbols"])


def test_save_and_load_skeleton(git_and_settings):
    git, settings = git_and_settings
    skel = build_skeleton(settings, "r", git, "main")
    save_skeleton(settings.data_dir, "r", "main", skel)
    loaded = load_skeleton(settings.data_dir, "r", "main")
    assert loaded["commit"] == skel["commit"]
    assert load_skeleton(settings.data_dir, "r", "nope") is None


async def test_get_overview_and_docs_tools(git_and_settings):
    git, settings = git_and_settings
    skel = build_skeleton(settings, "r", git, "main")
    save_skeleton(settings.data_dir, "r", "main", skel)

    registry = IndexerConfig(repos=[RepoConfig(id="r", path="r", primary_branch="main")])
    ctx = IndexerContext(settings, registry)

    ov = json.loads(await dispatch(ctx, "get_overview", {"repo": "r", "format": "json"}))
    assert ov["file_count"] == 3
    assert "top_symbols" in ov

    docs = json.loads(await dispatch(ctx, "get_docs", {"repo": "r", "format": "json"}))
    assert any(s["name"] == "alpha" for s in docs["results"])


async def test_get_overview_without_docs(git_and_settings):
    _git, settings = git_and_settings
    registry = IndexerConfig(repos=[RepoConfig(id="r", path="r")])
    ctx = IndexerContext(settings, registry)
    out = json.loads(await dispatch(ctx, "get_overview", {"repo": "r", "format": "json"}))
    assert "no docs" in out["error"]
