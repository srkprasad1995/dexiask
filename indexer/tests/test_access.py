"""Per-user repo gating: URL parsing, allowed-set resolution, and search enforcement."""
from __future__ import annotations

import indexer.github as gh
from indexer.access import RequestAuth, _auth, allowed_repo_ids, filter_allowed
from indexer.config import IndexerConfig, RepoConfig
from indexer.context import IndexerContext
from indexer.github import parse_github_url, repos_for_token
from indexer.mcp.tools import _search_hits
from indexer.pipeline import InMemoryStateStore
from indexer.pipeline.indexer import Pipeline
from indexer.settings import Settings
from indexer.store import QdrantStore

from .fakes import FakeProvider


def test_parse_github_url():
    assert parse_github_url("https://github.com/octo/hello.git") == ("octo", "hello")
    assert parse_github_url("https://github.com/octo/hello") == ("octo", "hello")
    assert parse_github_url("git@github.com:octo/hello.git") == ("octo", "hello")
    assert parse_github_url("https://gitlab.com/octo/hello") is None
    assert parse_github_url("") is None


def _ctx(tmp_path, repos, internal_token="secret"):
    settings = Settings(
        workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"),
        internal_token=internal_token,
    )
    return IndexerContext(settings, IndexerConfig(repos=repos), store=None, embedder=None)


def _set_auth(ra):
    token = _auth.set(ra)
    return lambda: _auth.reset(token)


def test_gating_disabled_without_secret(tmp_path):
    ctx = _ctx(tmp_path, [RepoConfig(id="r", url="https://github.com/o/r")], internal_token="")
    assert allowed_repo_ids(ctx) is None  # no secret → unrestricted


def test_admin_unrestricted_requires_valid_secret(tmp_path):
    ctx = _ctx(tmp_path, [RepoConfig(id="r", url="https://github.com/o/r")])
    # Valid internal token + unrestricted → None (admin).
    reset = _set_auth(RequestAuth(internal_ok=True, unrestricted=True, user_token=None))
    try:
        assert allowed_repo_ids(ctx) is None
    finally:
        reset()
    # Unrestricted claim WITHOUT a valid internal token is NOT honoured.
    reset = _set_auth(RequestAuth(internal_ok=False, unrestricted=True, user_token=None))
    try:
        assert allowed_repo_ids(ctx) == frozenset()  # deny, cannot forge admin
    finally:
        reset()


def test_member_gated_by_token(tmp_path, monkeypatch):
    repos = [
        RepoConfig(id="ok", url="https://github.com/o/ok"),
        RepoConfig(id="no", url="https://github.com/o/no"),
        RepoConfig(id="local", path=str(tmp_path)),  # not token-validatable
    ]
    ctx = _ctx(tmp_path, repos)
    # The token can see only o/ok.
    monkeypatch.setattr(gh, "has_access", lambda tok, owner, repo: repo == "ok")
    reset = _set_auth(RequestAuth(internal_ok=False, unrestricted=False, user_token="gho_x"))
    try:
        allowed = allowed_repo_ids(ctx)
        assert allowed == frozenset({"ok"})  # 'no' denied, 'local' excluded
    finally:
        reset()


def test_repos_for_token_excludes_local_and_nongithub(tmp_path, monkeypatch):
    repos = [
        RepoConfig(id="gh", url="https://github.com/o/gh"),
        RepoConfig(id="gl", url="https://gitlab.com/o/gl"),
        RepoConfig(id="local", path=str(tmp_path)),
    ]
    ctx = _ctx(tmp_path, repos)
    monkeypatch.setattr(gh, "has_access", lambda *a: True)  # token allows everything checkable
    assert repos_for_token(ctx, "tok") == frozenset({"gh"})  # only the GitHub repo


def test_search_restricted_to_allowed_repos(tmp_path, monkeypatch):
    # Two repos, each with one embedded doc; the member may only read 'ok'.
    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"), internal_token="secret")
    repos = [
        RepoConfig(id="ok", url="https://github.com/o/ok", primary_branch="main"),
        RepoConfig(id="no", url="https://github.com/o/no", primary_branch="main"),
    ]
    store = QdrantStore(location=":memory:")
    embedder = FakeProvider(dim=16)
    ctx = IndexerContext(settings, IndexerConfig(repos=repos), store=store, embedder=embedder)
    pipeline = Pipeline(settings, store, embedder, InMemoryStateStore())
    from indexer.docs import DomainDoc

    for rid in ("ok", "no"):
        pipeline.embed_docs(next(r for r in repos if r.id == rid), [
            DomainDoc("Payments", "concept", "payments", "how payments and billing work"),
        ])

    monkeypatch.setattr(gh, "has_access", lambda tok, owner, repo: repo == "ok")
    reset = _set_auth(RequestAuth(internal_ok=False, unrestricted=False, user_token="gho_x"))
    try:
        hits = _search_hits(ctx, {"query": "payments billing"})
        assert hits, "expected a hit from the allowed repo"
        assert {rid for rid, _ in hits} == {"ok"}, "must not leak the denied repo"
    finally:
        reset()

    # filter_allowed is a no-op when unrestricted.
    reset = _set_auth(RequestAuth(internal_ok=True, unrestricted=True, user_token=None))
    try:
        assert set(filter_allowed(ctx, ["ok", "no"])) == {"ok", "no"}
    finally:
        reset()
