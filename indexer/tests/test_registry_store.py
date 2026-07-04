"""Persisted repo registry: in-memory and FS stores, and restart durability."""
from __future__ import annotations

import pytest

from indexer.config import RepoConfig
from indexer.registry_store import FsRepoStore, InMemoryRepoStore


@pytest.fixture
def fs_store(tmp_path):
    return FsRepoStore(tmp_path / "repos.json")


@pytest.mark.parametrize("store_factory", [InMemoryRepoStore, "fs"])
def test_upsert_list_delete_roundtrip(store_factory, fs_store):
    store = fs_store if store_factory == "fs" else store_factory()

    store.upsert(RepoConfig(id="flask", url="https://github.com/pallets/flask.git",
                            primary_branch="main", include=["src/**"]))
    repos = store.list()
    assert len(repos) == 1
    got = repos[0]
    assert got.id == "flask"
    assert got.url.endswith("flask.git")
    assert got.include == ["src/**"]  # list field survives round-trip

    # upsert replaces in place (no duplicate).
    store.upsert(RepoConfig(id="flask", url="https://example.com/flask.git"))
    assert len(store.list()) == 1
    assert store.list()[0].url.endswith("example.com/flask.git")

    assert store.delete("flask") is True
    assert store.list() == []
    assert store.delete("flask") is False  # already gone


def test_fs_store_persists_across_new_instances(tmp_path):
    path = tmp_path / "repos.json"
    FsRepoStore(path).upsert(RepoConfig(id="x", path="x"))
    # A fresh store against the same file = a "restart".
    assert [r.id for r in FsRepoStore(path).list()] == ["x"]


def test_fs_store_missing_file_is_empty(tmp_path):
    assert FsRepoStore(tmp_path / "absent.json").list() == []
