"""End-to-end index orchestration: mirror fetch + reconcile + lock."""
from pathlib import Path

import pytest

from indexer.config import IndexerConfig, RepoConfig
from indexer.lock import InMemoryLock
from indexer.pipeline import InMemoryStateStore
from indexer.service import IndexService
from indexer.settings import Settings
from indexer.store import QdrantStore

from .fakes import FakeProvider


@pytest.fixture
def service(sample_repo: Path, tmp_path: Path):
    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    registry = IndexerConfig(
        repos=[RepoConfig(id="r", path=str(sample_repo), primary_branch="main")]
    )
    store = QdrantStore(location=":memory:")
    return IndexService(
        settings, registry, store=store, embedder=FakeProvider(dim=16),
        state=InMemoryStateStore(), lock=InMemoryLock(),
    ), store, registry


def test_index_repo_indexes_primary_branch(service):
    svc, store, _ = service
    out = svc.index_repo("r")
    assert out["status"] == "indexed"
    assert out["branch"] == "main"
    assert out["embedded"] == 3
    assert store.count("chunks_r") == 3
    # The mirror is single-branch → only "main" is present.
    assert set(svc._mirror(_repo(svc)).repo().branches()) == {"main"}


def _repo(svc):
    return svc.registry.repos[0]


def test_index_repo_reindex_is_idempotent(service):
    svc, store, _ = service
    svc.index_repo("r")
    out = svc.index_repo("r")
    assert out["embedded"] == 0  # nothing changed
    assert store.count("chunks_r") == 3


def test_index_repo_unknown_raises(service):
    svc, _, _ = service
    from indexer.context import RepoNotFoundError
    with pytest.raises(RepoNotFoundError):
        svc.index_repo("nope")


def test_lock_blocks_concurrent_same_repo(sample_repo, tmp_path):
    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    registry = IndexerConfig(repos=[RepoConfig(id="r", path=str(sample_repo))])
    lock = InMemoryLock()
    svc = IndexService(
        settings, registry, store=QdrantStore(location=":memory:"),
        embedder=FakeProvider(dim=16), state=InMemoryStateStore(), lock=lock,
    )
    with lock.acquire("index:r") as held:
        assert held
        out = svc.index_repo("r")
        assert out["status"] == "already_running"


def test_degraded_without_pipeline(sample_repo, tmp_path):
    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    registry = IndexerConfig(repos=[RepoConfig(id="r", path=str(sample_repo))])
    svc = IndexService(
        settings, registry, store=None, embedder=None,
        state=InMemoryStateStore(), lock=InMemoryLock(),
    )
    out = svc.index_repo("r")
    assert out["status"] == "mirror_refreshed"
    assert out["semantic_index"] == "pending"
