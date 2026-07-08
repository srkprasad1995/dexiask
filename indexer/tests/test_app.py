from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from indexer.config import IndexerConfig, RepoConfig
from indexer.context import IndexerContext
from indexer.settings import Settings

from .fakes import FakeProvider


@pytest.fixture
def app_ctx(sample_repo: Path, tmp_path: Path):
    """The context + service the client is built on, exposed so tests can seed the
    shared progress store the status endpoints read from."""
    from indexer.lock import InMemoryLock
    from indexer.pipeline import InMemoryStateStore
    from indexer.service import IndexService
    from indexer.store import QdrantStore

    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    registry = IndexerConfig(
        repos=[RepoConfig(id="r", path=str(sample_repo), primary_branch="main")]
    )
    store = QdrantStore(location=":memory:")
    ctx = IndexerContext(settings, registry, store=store, embedder=FakeProvider(dim=16))
    service = IndexService(
        settings, registry, store=store, embedder=FakeProvider(dim=16),
        state=InMemoryStateStore(), lock=InMemoryLock(),
    )
    return ctx, service


@pytest.fixture
def service(app_ctx):
    return app_ctx[1]


@pytest.fixture
def client(app_ctx) -> TestClient:
    from indexer.app import create_app

    ctx, service = app_ctx
    return TestClient(create_app(ctx, service))


def test_healthz(client):
    assert client.get("/healthz").json() == {"status": "ok"}


def test_status_before_index(client):
    body = client.get("/v1/status").json()
    assert body["repos"][0]["id"] == "r"
    assert body["repos"][0]["indexed"] is False


def test_reindex_then_status(client):
    r = client.post("/reindex", json={"repo": "r"})
    results = r.json()["results"]
    assert results[0]["repo"] == "r"
    assert results[0]["status"] == "indexed"
    assert results[0]["embedded"] == 3

    body = client.get("/v1/status").json()
    entry = body["repos"][0]
    assert entry["indexed"] is True
    assert entry["branch"] == "main"
    assert entry["commit"]


def test_reindex_unknown_repo(client):
    assert client.post("/reindex", json={"repo": "nope"}).status_code == 404


def test_status_omits_progress_when_idle(client):
    # No active run → no status/percent fields, just the plain indexed flag.
    entry = client.get("/v1/status").json()["repos"][0]
    assert "status" not in entry
    assert "percent" not in entry


def test_status_and_repos_report_active_progress(client, service):
    from indexer.pipeline.progress import PHASE_EMBEDDING

    # Simulate a run mid-embed by seeding the shared progress store the endpoints read.
    service.progress.begin("r", PHASE_EMBEDDING)
    service.progress.set_total("r", 8)
    service.progress.advance("r", 6)

    for path in ("/v1/status", "/v1/repos"):
        entry = client.get(path).json()["repos"][0]
        assert entry["status"] == PHASE_EMBEDDING
        assert entry["percent"] == 75  # 6/8


def test_repos_progress_percent_only_while_embedding(client, service):
    from indexer.pipeline.progress import PHASE_CLONING

    # Non-embedding phases report the phase but carry no percent.
    service.progress.begin("r", PHASE_CLONING)
    entry = client.get("/v1/repos").json()["repos"][0]
    assert entry["status"] == PHASE_CLONING
    assert "percent" not in entry


def test_status_reports_ready(client):
    assert client.get("/v1/status").json()["status"] == "ready"


def test_search_requires_query(client):
    r = client.post("/v1/search", json={})
    assert r.status_code == 400
    assert r.json()["results"] == []


def test_search_returns_json_results(client):
    client.post("/reindex", json={"repo": "r"})
    r = client.post("/v1/search", json={"query": "hello", "limit": 5})
    assert r.status_code == 200
    results = r.json()["results"]
    assert isinstance(results, list) and results
    row = results[0]
    # Clean camelCase shape the web indexer page renders directly.
    assert set(row) >= {"repo", "path", "score", "content"}
    assert row["repo"] == "r"
    assert "startLine" in row and "endLine" in row


def test_register_and_deregister_repo(client):
    client.post("/v1/repos", json={
        "id": "extern", "url": "https://github.com/o/extern.git", "primary_branch": "main",
    })
    ids = {r["id"] for r in client.get("/v1/status").json()["repos"]}
    assert "extern" in ids

    resp = client.delete("/v1/repos/extern").json()
    assert resp == {"deregistered": "extern", "removed": True}
    ids2 = {r["id"] for r in client.get("/v1/status").json()["repos"]}
    assert "extern" not in ids2


def test_list_repos_returns_all_repos_with_status(client):
    # Every registered repo is listed (path-only and URL), each with its index
    # status, so the web indexer page can render one row per repo.
    client.post("/v1/repos", json={
        "id": "extern", "url": "https://github.com/o/extern.git", "primary_branch": "develop",
    })
    repos = client.get("/v1/repos").json()["repos"]
    by_id = {r["id"]: r for r in repos}
    # The path-only fixture repo is now included (with its /workspace path).
    assert "r" in by_id
    assert by_id["r"].get("path")
    assert by_id["r"]["indexed"] is False
    # The URL repo carries its clone URL and primary branch.
    assert by_id["extern"]["url"] == "https://github.com/o/extern.git"
    assert by_id["extern"]["branch"] == "develop"


def test_registered_repo_survives_restart(sample_repo, tmp_path):
    """A repo registered via the store is reloaded on rebuild (FS-backed, no DB)."""
    from indexer.app import build_components
    from indexer.registry_store import FsRepoStore

    settings = Settings(
        workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"),
        repos_config_path=str(tmp_path / "absent.yaml"),  # empty base registry
    )
    _ctx, _svc, store = build_components(settings)
    assert isinstance(store, FsRepoStore)
    store.upsert(RepoConfig(id="persisted", url="https://github.com/o/p.git", primary_branch="main"))

    # "Restart": a fresh build overlays the persisted repo onto the (empty) yaml.
    ctx2, _svc2, _store2 = build_components(settings)
    assert ctx2.registry.get("persisted") is not None
    assert ctx2.registry.get("persisted").url.endswith("p.git")


def test_git_token_status_before_set(client):
    assert client.get("/v1/git-token").json() == {"configured": False}


def test_git_token_set_marks_configured(client, tmp_path):
    r = client.put("/v1/git-token", json={"token": "abc"})
    assert r.status_code == 200
    assert r.json() == {"configured": True}
    # GET still never returns the value, only the configured flag.
    assert client.get("/v1/git-token").json() == {"configured": True}
    # The token file is persisted with owner-only permissions.
    token_file = tmp_path / "data" / "git_token"
    assert token_file.read_text() == "abc"
    assert (token_file.stat().st_mode & 0o777) == 0o600


def test_git_token_clear(client, tmp_path):
    client.put("/v1/git-token", json={"token": "abc"})
    r = client.put("/v1/git-token", json={"token": ""})
    assert r.json() == {"configured": False}
    assert client.get("/v1/git-token").json() == {"configured": False}
    assert not (tmp_path / "data" / "git_token").exists()


def test_git_token_mutation_visible_to_service(sample_repo, tmp_path):
    """A UI-set token mutates the shared Settings so reindex/reconcile use it."""
    from fastapi.testclient import TestClient

    from indexer.app import create_app
    from indexer.lock import InMemoryLock
    from indexer.pipeline import InMemoryStateStore
    from indexer.service import IndexService
    from indexer.store import QdrantStore

    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    registry = IndexerConfig(
        repos=[RepoConfig(id="r", path=str(sample_repo), primary_branch="main")]
    )
    store = QdrantStore(location=":memory:")
    ctx = IndexerContext(settings, registry, store=store, embedder=FakeProvider(dim=16))
    service = IndexService(
        settings, registry, store=store, embedder=FakeProvider(dim=16),
        state=InMemoryStateStore(), lock=InMemoryLock(),
    )
    # ctx and service share ONE Settings object, so a token set via the endpoint
    # is visible to the indexing path (build_repo_mirror reads settings.git_token).
    assert ctx.settings is service.settings
    c = TestClient(create_app(ctx, service))
    c.put("/v1/git-token", json={"token": "tok-123"})
    assert service.settings.git_token == "tok-123"


def test_reindex_git_error_returns_502_with_hint(sample_repo, tmp_path):
    """A clone/fetch failure surfaces a clean 502 + actionable hint, not a 500."""
    from fastapi.testclient import TestClient

    from indexer.app import create_app
    from indexer.git.repo import GitError
    from indexer.lock import InMemoryLock
    from indexer.pipeline import InMemoryStateStore
    from indexer.service import IndexService
    from indexer.store import QdrantStore

    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    registry = IndexerConfig(
        repos=[RepoConfig(id="r", path=str(sample_repo), primary_branch="main")]
    )
    store = QdrantStore(location=":memory:")
    ctx = IndexerContext(settings, registry, store=store, embedder=FakeProvider(dim=16))
    service = IndexService(
        settings, registry, store=store, embedder=FakeProvider(dim=16),
        state=InMemoryStateStore(), lock=InMemoryLock(),
    )

    def boom(repo_id, **kw):
        raise GitError(
            "git clone ... failed: fatal: could not read Username for 'https://github.com'"
        )

    service.index_repo = boom  # type: ignore[method-assign]
    c = TestClient(create_app(ctx, service))

    resp = c.post("/reindex", json={"repo": "r"})
    assert resp.status_code == 502
    body = resp.json()
    assert body["repo"] == "r"
    assert "git token" in body["error"] or "sign in" in body["error"]


def test_domain_docs_endpoint(client, tmp_path):
    from indexer.docs import save_domain_docs

    # Unknown repo → 404.
    assert client.get("/v1/docs/nope").status_code == 404

    # Empty until generated.
    assert client.get("/v1/docs/r").json() == {"docs": []}

    # After a save, the endpoint serves them for the web Docs tab.
    save_domain_docs(
        str(tmp_path / "data"), "r", "main",
        [{"title": "Arch", "category": "architecture", "slug": "arch", "body": "b"}],
    )
    docs = client.get("/v1/docs/r").json()["docs"]
    assert docs[0]["title"] == "Arch"


def test_index_uses_x_git_token_header_fallback(sample_repo, tmp_path):
    """When no body token is given, reindex/index use the X-Git-Token header."""
    from fastapi.testclient import TestClient

    from indexer.app import create_app
    from indexer.lock import InMemoryLock
    from indexer.pipeline import InMemoryStateStore
    from indexer.service import IndexService
    from indexer.store import QdrantStore

    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    registry = IndexerConfig(
        repos=[RepoConfig(id="r", path=str(sample_repo), primary_branch="main")]
    )
    store = QdrantStore(location=":memory:")
    ctx = IndexerContext(settings, registry, store=store, embedder=FakeProvider(dim=16))
    service = IndexService(
        settings, registry, store=store, embedder=FakeProvider(dim=16),
        state=InMemoryStateStore(), lock=InMemoryLock(),
    )

    captured: list[str | None] = []

    def spy(repo_id, **kw):
        captured.append(kw.get("token"))
        return {"repo": repo_id, "status": "indexed", "embedded": 0}

    service.index_repo = spy  # type: ignore[method-assign]
    c = TestClient(create_app(ctx, service))

    # Header used when body has no token.
    c.post("/v1/index/r", headers={"X-Git-Token": "hdr-tok"})
    assert captured[-1] == "hdr-tok"

    # An explicit body token still wins over the header.
    c.post("/v1/index/r", json={"token": "body-tok"}, headers={"X-Git-Token": "hdr-tok"})
    assert captured[-1] == "body-tok"

    # No token anywhere → None.
    c.post("/reindex", json={"repo": "r"})
    assert captured[-1] is None


def test_git_token_persisted_across_rebuild(tmp_path):
    """A freshly build_components-built app loads the persisted token."""
    from indexer.app import build_components

    settings = Settings(
        workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"),
        repos_config_path=str(tmp_path / "absent.yaml"),
    )
    from indexer.app import write_git_token

    write_git_token(settings, "persisted-tok")

    # A fresh Settings (no env token) picks up the persisted file on rebuild.
    fresh = Settings(
        workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"),
        repos_config_path=str(tmp_path / "absent.yaml"),
    )
    ctx, service, _store = build_components(fresh)
    assert ctx.settings.git_token == "persisted-tok"
    assert service.settings.git_token == "persisted-tok"


def test_mcp_endpoint_initializes(client):
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    with client as c:
        init = c.post("/mcp/", headers=headers, json={
            "jsonrpc": "2.0", "id": 1, "method": "initialize",
            "params": {
                "protocolVersion": "2025-06-18",
                "capabilities": {},
                "clientInfo": {"name": "test", "version": "0"},
            },
        })
    assert init.status_code == 200


def test_list_tools_endpoint(client):
    body = client.get("/v1/tools").json()
    names = {t["name"] for t in body["tools"]}
    assert {"semantic_search", "lexical_search", "git_log", "get_overview"} <= names
    assert all("input_schema" in t for t in body["tools"])


def test_run_tool_endpoint(client):
    client.post("/reindex", json={"repo": "r"})  # ensure mirror exists
    body = client.post("/v1/tools/list_repos", json={"format": "json"}).json()
    assert body["tool"] == "list_repos"
    assert '"id"' in body["result"]


def test_run_tool_unknown(client):
    body = client.post("/v1/tools/nope", json={"format": "json"}).json()
    assert "unknown tool" in body["result"]
