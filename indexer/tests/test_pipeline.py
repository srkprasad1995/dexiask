from pathlib import Path

import pytest

from indexer.config import RepoConfig
from indexer.git.mirror import Mirror
from indexer.pipeline import InMemoryStateStore, Pipeline
from indexer.settings import Settings
from indexer.store import QdrantStore

from .fakes import FakeProvider


@pytest.fixture
def setup(sample_repo: Path, tmp_path: Path):
    mirror = Mirror(tmp_path / "m" / "r.git", sample_repo, branch="main")
    mirror.ensure()
    git = mirror.repo()
    repo = RepoConfig(id="r", path=str(sample_repo), primary_branch="main")
    emb = FakeProvider(dim=16)
    store = QdrantStore(location=":memory:")
    pipe = Pipeline(Settings(), store, emb, InMemoryStateStore())
    return pipe, repo, git, store, emb


def test_reconcile_embeds_all(setup):
    pipe, repo, git, store, _ = setup
    res = pipe.reconcile(repo, git)
    assert res.embedded == 3  # a.py, b.py, c.py (one chunk each)
    assert res.removed == 0
    assert store.count(repo.collection) == 3


def test_reconcile_same_commit_is_noop(setup):
    pipe, repo, git, store, _ = setup
    pipe.reconcile(repo, git)
    res = pipe.reconcile(repo, git)
    assert (res.embedded, res.removed) == (0, 0)  # nothing changed → skipped
    assert store.count(repo.collection) == 3


def test_full_reindex_is_idempotent(setup):
    pipe, repo, git, store, _ = setup
    pipe.reconcile(repo, git, full=True)
    n1 = store.count(repo.collection)
    pipe.reconcile(repo, git, full=True)  # deterministic ids → idempotent
    assert store.count(repo.collection) == n1 == 3


def test_reconcile_repairs_lost_write(setup):
    pipe, repo, git, store, _ = setup
    pipe.reconcile(repo, git)
    # Simulate a lost write: drop one path's points + its state record.
    pid, payload = next(iter(store.scroll_by_path(repo.collection, "a.py")))
    store.delete(repo.collection, [pid])
    pipe.state.remove_path(repo.id, "a.py")
    assert store.count(repo.collection) == 2
    pipe.reconcile(repo, git)  # convergent repair
    assert store.count(repo.collection) == 3


def test_searchable_after_index(setup):
    pipe, repo, git, store, emb = setup
    pipe.reconcile(repo, git)
    q = emb.embed_query("alpha")
    assert store.search(repo.collection, q, limit=5)


def test_exclude_filters_paths(sample_repo, tmp_path):
    mirror = Mirror(tmp_path / "m" / "r.git", sample_repo, branch="main")
    mirror.ensure()
    repo = RepoConfig(id="r", path=str(sample_repo), exclude=["c.py"])
    pipe = Pipeline(Settings(), QdrantStore(location=":memory:"), FakeProvider(dim=16), InMemoryStateStore())
    res = pipe.reconcile(repo, mirror.repo())
    assert res.embedded == 2  # c.py excluded
    assert pipe.store.count(repo.collection) == 2


def test_removed_path_is_gced(sample_repo, tmp_path):
    """A path that leaves the tree is deleted from the index on the next reconcile."""
    mirror = Mirror(tmp_path / "m" / "r.git", sample_repo, branch="main")
    mirror.ensure()
    repo = RepoConfig(id="r", path=str(sample_repo), primary_branch="main")
    store = QdrantStore(location=":memory:")
    state = InMemoryStateStore()
    pipe = Pipeline(Settings(), store, FakeProvider(dim=16), state)
    pipe.reconcile(repo, mirror.repo())
    assert store.count(repo.collection) == 3
    # Now exclude c.py and reconcile: it must be removed.
    repo2 = RepoConfig(id="r", path=str(sample_repo), primary_branch="main", exclude=["c.py"])
    res = pipe.reconcile(repo2, mirror.repo())
    assert res.removed == 1
    assert store.count(repo.collection) == 2
