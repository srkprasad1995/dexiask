from indexer.pipeline.progress import (
    PHASE_CLONING,
    PHASE_EMBEDDING,
    ProgressStore,
)


def test_snapshot_none_until_begun():
    store = ProgressStore()
    assert store.snapshot("r") is None


def test_begin_set_total_advance():
    store = ProgressStore()
    store.begin("r", PHASE_CLONING)
    assert store.snapshot("r").phase == PHASE_CLONING

    store.set_phase("r", PHASE_EMBEDDING)
    store.set_total("r", 10)
    store.advance("r")
    store.advance("r", 3)
    p = store.snapshot("r")
    assert p.phase == PHASE_EMBEDDING
    assert p.total == 10
    assert p.processed == 4


def test_set_phase_resets_counters():
    store = ProgressStore()
    store.begin("r", PHASE_EMBEDDING)
    store.set_total("r", 5)
    store.advance("r", 5)
    store.set_phase("r", PHASE_CLONING)
    p = store.snapshot("r")
    assert p.processed == 0
    assert p.total == 0


def test_clear_removes_entry():
    store = ProgressStore()
    store.begin("r", PHASE_CLONING)
    store.clear("r")
    assert store.snapshot("r") is None


def test_snapshot_is_a_copy():
    store = ProgressStore()
    store.begin("r", PHASE_EMBEDDING)
    store.set_total("r", 2)
    snap = store.snapshot("r")
    store.advance("r")
    # The earlier snapshot is unaffected by later mutation.
    assert snap.processed == 0


def test_updates_on_missing_repo_are_ignored():
    store = ProgressStore()
    # No begin() → these are no-ops rather than errors.
    store.set_total("r", 5)
    store.advance("r")
    assert store.snapshot("r") is None
