import json
from pathlib import Path

import pytest

from indexer.config import IndexerConfig, RepoConfig
from indexer.context import IndexerContext
from indexer.mcp.server import build_mcp_server, build_session_manager
from indexer.mcp.tools import build_tools, dispatch
from indexer.settings import Settings
from indexer.store import Chunk, QdrantStore, build_point

from .fakes import FakeProvider


@pytest.fixture
def ctx(sample_repo: Path, tmp_path: Path) -> IndexerContext:
    settings = Settings(
        workspace_root=str(tmp_path),
        data_dir=str(tmp_path / "data"),
        embedding_dim=16,
    )
    registry = IndexerConfig(
        repos=[RepoConfig(id="r", path=str(sample_repo), primary_branch="main")]
    )
    emb = FakeProvider(dim=16)
    store = QdrantStore(location=":memory:")
    context = IndexerContext(settings, registry, store=store, embedder=emb)
    context.mirror_for("r").ensure()  # single-branch (main) mirror for read tools
    return context


async def test_list_repos(ctx):
    out = await dispatch(ctx, "list_repos", {"format": "json"})
    assert json.loads(out)["results"][0]["id"] == "r"


async def test_glob(ctx):
    out = await dispatch(ctx, "glob", {"repo": "r", "pattern": "*.py", "format": "json"})
    paths = {row["path"] for row in json.loads(out)["results"]}
    assert paths == {"a.py", "b.py", "c.py"}


async def test_lexical_search(ctx):
    out = await dispatch(ctx, "lexical_search", {"repo": "r", "pattern": "return 10", "format": "json"})
    rows = json.loads(out)["results"]
    assert any(r["path"] == "a.py" for r in rows)


async def test_read_range(ctx):
    out = await dispatch(ctx, "read_range", {"repo": "r", "path": "a.py"})
    assert "return 10" in out
    assert "a.py@main" in out


async def test_get_chunk(ctx):
    out = await dispatch(ctx, "get_chunk", {"repo": "r", "path": "a.py"})
    assert "return 10" in out


async def test_explore(ctx):
    out = await dispatch(ctx, "explore", {"repo": "r", "format": "json"})
    entries = {row["entry"] for row in json.loads(out)["results"]}
    assert {"a.py", "b.py", "c.py"} <= entries


async def test_git_branches_and_log(ctx):
    out = await dispatch(ctx, "git_branches", {"repo": "r", "format": "json"})
    assert {r["branch"] for r in json.loads(out)["results"]} == {"main"}  # single-branch mirror
    log = await dispatch(ctx, "git_log", {"repo": "r", "format": "json"})
    assert json.loads(log)["results"][0]["subject"] == "C2"


async def test_git_file_at_rev(ctx):
    out = await dispatch(ctx, "git_file_at_rev", {"repo": "r", "path": "a.py", "rev": "main"})
    assert "return 10" in out


async def test_semantic_search_with_index(ctx):
    repo = ctx.repo_config("r")
    ctx.store.ensure_collection(repo.collection, ctx.embedder.dim)
    chunk = Chunk(blob_sha="x", ordinal=0, text="def alpha returns ten", lang="py",
                  symbol="alpha", symbol_kind="function", start_line=1, end_line=2)
    vec = ctx.embedder.embed_documents([chunk.text])[0]
    ctx.store.upsert(repo.collection, [build_point("r", chunk, vec, "a.py")])

    out = await dispatch(ctx, "semantic_search", {"repo": "r", "query": "alpha", "format": "json"})
    rows = json.loads(out)["results"]
    assert rows and rows[0]["symbol"] == "alpha"
    assert rows[0]["path"] == "a.py"
    assert rows[0]["lines"] == "1-2"


def _seed_chunk(ctx, *, text, lang, path, symbol="", kind=""):
    repo = ctx.repo_config("r")
    ctx.store.ensure_collection(repo.collection, ctx.embedder.dim)
    c = Chunk(blob_sha=path, ordinal=0, text=text, lang=lang, symbol=symbol,
              symbol_kind=kind, start_line=1, end_line=4)
    v = ctx.embedder.embed_documents([text])[0]
    ctx.store.upsert(repo.collection, [build_point("r", c, v, path)])


async def test_semantic_search_code_only_drops_noise(ctx):
    _seed_chunk(ctx, text="alpha signing helper", lang="python",
                path="signer.py", symbol="sign", kind="function")
    _seed_chunk(ctx, text="alpha signing helper", lang="text", path=".gitignore")

    out = json.loads(await dispatch(
        ctx, "semantic_search", {"repo": "r", "query": "alpha signing", "format": "json"}))
    rows = out["results"]
    assert [r["path"] for r in rows] == ["signer.py"]
    assert rows[0]["lang"] == "python"
    assert rows[0]["symbol_kind"] == "function"
    assert "alpha signing helper" in rows[0]["code"]


async def test_semantic_search_code_only_false_includes_noise(ctx):
    _seed_chunk(ctx, text="alpha helper", lang="python", path="a.py")
    _seed_chunk(ctx, text="alpha helper", lang="text", path=".gitignore")

    out = json.loads(await dispatch(
        ctx, "semantic_search",
        {"repo": "r", "query": "alpha", "code_only": False, "format": "json"}))
    assert {r["path"] for r in out["results"]} == {"a.py", ".gitignore"}


async def test_semantic_search_min_score_filters(ctx):
    _seed_chunk(ctx, text="alpha helper", lang="python", path="a.py")
    base = json.loads(await dispatch(
        ctx, "semantic_search", {"repo": "r", "query": "alpha", "format": "json"}))
    assert base["results"]
    high = json.loads(await dispatch(
        ctx, "semantic_search",
        {"repo": "r", "query": "alpha", "min_score": 2.0, "format": "json"}))
    assert high["results"] == []  # cosine can't exceed 1.0


async def test_semantic_search_degrades_without_store(sample_repo, tmp_path):
    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "d"))
    registry = IndexerConfig(repos=[RepoConfig(id="r", path=str(sample_repo))])
    ctx = IndexerContext(settings, registry)  # no store/embedder
    out = await dispatch(ctx, "semantic_search", {"repo": "r", "query": "x", "format": "json"})
    assert "unavailable" in json.loads(out)["error"]


async def test_semantic_search_without_embedder_names_keys_and_fallback(sample_repo, tmp_path):
    """No embeddings provider → the message names every way to configure one
    (hosted keys AND the local sidecar) plus the lexical fallback, so the agent
    doesn't dead-end on a bare 'unavailable' and instead switches to
    lexical_search in the same turn."""
    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "d"))
    registry = IndexerConfig(repos=[RepoConfig(id="r", path=str(sample_repo))])
    ctx = IndexerContext(settings, registry)  # no embedder (no provider resolved)
    out = await dispatch(ctx, "semantic_search", {"repo": "r", "query": "x", "format": "json"})
    msg = json.loads(out)["error"].lower()
    assert "voyage_api_key" in msg and "openai_api_key" in msg
    assert "compose_profiles=local" in msg
    assert "lexical_search" in msg


async def test_unknown_tool(ctx):
    out = await dispatch(ctx, "nope", {"format": "json"})
    assert "unknown tool" in json.loads(out)["error"]


async def test_repo_not_indexed_error(ctx):
    ctx.registry.repos.append(RepoConfig(id="missing", path="missing"))
    out = await dispatch(ctx, "glob", {"repo": "missing", "pattern": "*", "format": "json"})
    assert "no mirror yet" in json.loads(out)["error"]


def test_mcp_server_builds(ctx):
    server = build_mcp_server(ctx)
    assert server.name == "dexiask-indexer"
    names = {t.name for t in build_tools()}
    assert {"semantic_search", "lexical_search", "glob", "read_range", "git_log"} <= names
    # The removed multi-branch tools are gone.
    assert "index_branch" not in names
    assert "index_status" not in names


def test_session_manager_builds(ctx):
    assert build_session_manager(ctx) is not None
