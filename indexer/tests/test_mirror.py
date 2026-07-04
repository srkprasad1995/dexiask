"""Mirror sourcing: local paths, remote URLs, single-branch clone, token auth."""
from __future__ import annotations

import base64
from pathlib import Path

import pytest

from indexer.config import IndexerConfig, RepoConfig
from indexer.git import mirror as mirror_mod
from indexer.git.mirror import Mirror, build_auth_header, build_repo_mirror, is_remote_url
from indexer.lock import InMemoryLock
from indexer.pipeline import InMemoryStateStore
from indexer.service import IndexService
from indexer.settings import Settings
from indexer.store import QdrantStore

from .fakes import FakeProvider


@pytest.mark.parametrize(
    "url,expected",
    [
        ("https://github.com/o/r.git", True),
        ("http://host/r.git", True),
        ("ssh://git@host/r.git", True),
        ("git@github.com:o/r.git", True),
        ("file:///tmp/r", True),
        ("/local/path/repo", False),
        ("relative/repo", False),
    ],
)
def test_is_remote_url(url, expected):
    assert is_remote_url(url) is expected


def test_build_auth_header_basic_encoding():
    header = build_auth_header("tok123", "x-access-token")
    assert header.startswith("Authorization: Basic ")
    raw = base64.b64decode(header.removeprefix("Authorization: Basic ")).decode()
    assert raw == "x-access-token:tok123"


def test_build_auth_header_empty_token_is_none():
    assert build_auth_header("") is None


def _settings(tmp_path: Path, **kw) -> Settings:
    return Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"), **kw)


def test_build_repo_mirror_local_has_no_auth(tmp_path):
    repo = RepoConfig(id="r", path="r")
    m = build_repo_mirror(_settings(tmp_path), repo)
    assert m._auth is None
    assert not is_remote_url(m.source)
    assert m.branch == "main"


def test_mirror_clones_single_branch(sample_repo, tmp_path):
    """The mirror tracks only the primary branch; others are absent."""
    m = Mirror(tmp_path / "lim.git", sample_repo, branch="main")
    m.ensure()
    assert set(m.repo().branches()) == {"main"}  # feat/copy excluded
    m.fetch()  # refresh stays limited to the tracked branch
    assert set(m.repo().branches()) == {"main"}


def test_build_repo_mirror_uses_repo_primary_branch(tmp_path):
    repo = RepoConfig(id="r", path="r", primary_branch="develop")
    m = build_repo_mirror(_settings(tmp_path), repo)
    assert m.branch == "develop"


def test_build_repo_mirror_https_uses_token(tmp_path):
    repo = RepoConfig(id="r", url="https://github.com/o/r.git")
    m = build_repo_mirror(_settings(tmp_path, git_token="envtok"), repo)
    assert m._auth and "Basic" in m._auth
    assert m.source == "https://github.com/o/r.git"


def test_build_repo_mirror_per_request_token_overrides_env(tmp_path):
    repo = RepoConfig(id="r", url="https://github.com/o/r.git")
    m = build_repo_mirror(_settings(tmp_path, git_token="envtok"), repo, token="reqtok")
    decoded = base64.b64decode(m._auth.split("Basic ")[1]).decode()
    assert decoded.endswith(":reqtok")


def test_build_repo_mirror_https_without_token_has_no_auth(tmp_path):
    repo = RepoConfig(id="r", url="https://github.com/o/r.git")
    m = build_repo_mirror(_settings(tmp_path), repo)
    assert m._auth is None


def test_build_repo_mirror_ssh_url_gets_no_http_auth(tmp_path):
    repo = RepoConfig(id="r", url="ssh://git@github.com/o/r.git")
    m = build_repo_mirror(_settings(tmp_path, git_token="envtok"), repo)
    assert m._auth is None  # token only applies to http(s)


def test_git_injects_auth_header_via_env(monkeypatch):
    captured = {}

    class _Proc:
        returncode = 0
        stderr = b""

    def fake_run(args, capture_output, env):
        captured["args"] = args
        captured["env"] = env
        return _Proc()

    monkeypatch.setattr(mirror_mod.subprocess, "run", fake_run)
    mirror_mod._git("fetch", auth_header="Authorization: Basic abc")
    env = captured["env"]
    assert env["GIT_CONFIG_COUNT"] == "1"
    assert env["GIT_CONFIG_KEY_0"] == "http.extraHeader"
    assert env["GIT_CONFIG_VALUE_0"] == "Authorization: Basic abc"
    assert "abc" not in " ".join(captured["args"])  # token never in argv


def test_git_without_auth_sets_no_config_env(monkeypatch):
    captured = {}

    class _Proc:
        returncode = 0
        stderr = b""

    monkeypatch.setattr(
        mirror_mod.subprocess, "run",
        lambda args, capture_output, env: captured.update(env=env) or _Proc(),
    )
    mirror_mod._git("status")
    assert "GIT_CONFIG_COUNT" not in captured["env"]


def test_remote_index_end_to_end_via_file_url(sample_repo, tmp_path):
    """A repo configured by URL (file://) clones + indexes with no local path."""
    url = f"file://{sample_repo}"
    assert is_remote_url(url)
    settings = _settings(tmp_path)
    registry = IndexerConfig(repos=[RepoConfig(id="r", url=url, primary_branch="main")])
    store = QdrantStore(location=":memory:")
    svc = IndexService(
        settings, registry, store=store, embedder=FakeProvider(dim=16),
        state=InMemoryStateStore(), lock=InMemoryLock(),
    )
    out = svc.index_repo("r")
    assert out["status"] == "indexed"
    assert out["branch"] == "main"
    assert store.count("chunks_r") > 0
    assert build_repo_mirror(settings, registry.get("r")).exists()


def test_mirror_local_dotgit_resolution(tmp_path):
    work = tmp_path / "w"
    (work / ".git").mkdir(parents=True)
    m = Mirror(tmp_path / "mir.git", work)
    assert m.source.endswith("/.git")
