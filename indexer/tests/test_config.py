import pytest

from indexer.config import RepoConfig
from indexer.config.registry import load_registry, merge_repo


def test_load_registry_parses_repos():
    cfg = load_registry(
        """
        default_embedding:
          provider: voyage
          model: voyage-code-3
          dim: 1024
        repos:
          - id: dexiask
            path: dexiask
            primary_branch: main
          - id: web
            path: dexiask-web
            subpath: src
        """
    )
    assert len(cfg.repos) == 2
    assert cfg.get("dexiask").primary_branch == "main"
    assert cfg.get("web").subpath == "src"
    assert cfg.get("missing") is None


def test_collection_name():
    r = RepoConfig(id="dexiask", path="dexiask")
    assert r.collection == "chunks_dexiask"


def test_invalid_id_rejected():
    with pytest.raises(ValueError):
        RepoConfig(id="bad id!", path="x")


def test_duplicate_ids_rejected():
    with pytest.raises(ValueError):
        load_registry(
            """
            repos:
              - id: dup
                path: a
              - id: dup
                path: b
            """
        )


def test_merge_repo_replaces_by_id():
    cfg = load_registry("repos:\n  - id: a\n    path: a1\n")
    cfg2 = merge_repo(cfg, RepoConfig(id="a", path="a2"))
    assert len(cfg2.repos) == 1
    assert cfg2.get("a").path == "a2"


def test_embedding_for_falls_back_to_default():
    cfg = load_registry("repos:\n  - id: a\n    path: a\n")
    assert cfg.embedding_for(cfg.get("a")).model == "voyage-code-3"


def test_repo_accepts_url_without_path():
    r = RepoConfig(id="flask", url="https://github.com/pallets/flask.git")
    assert r.url.endswith("flask.git")
    assert r.path == ""
    assert r.collection == "chunks_flask"


def test_repo_requires_url_or_path():
    with pytest.raises(ValueError):
        RepoConfig(id="nope")


def test_load_registry_parses_url_repo():
    cfg = load_registry(
        """
        repos:
          - id: flask
            url: https://github.com/pallets/flask.git
            primary_branch: main
        """
    )
    assert cfg.get("flask").url.endswith("flask.git")
