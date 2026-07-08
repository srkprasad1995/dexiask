"""
Live index progress.

A reindex runs synchronously in a worker thread while the status endpoints read
from another thread, so the store is guarded by a lock. It holds an entry only
while a repo is actively indexing (``begin`` → ``clear``); ``snapshot`` returns
``None`` for an idle repo, which the endpoints use to fall back to the plain
``indexed`` state. In-memory and process-local, like the state store and lock.
"""
from __future__ import annotations

import threading
from dataclasses import dataclass, replace

# Phases, in the order a pass moves through them.
PHASE_CLONING = "cloning"
PHASE_DOCS = "docs"
PHASE_EMBEDDING = "embedding"


@dataclass
class RepoProgress:
    phase: str
    processed: int = 0
    total: int = 0


class ProgressStore:
    def __init__(self) -> None:
        self._active: dict[str, RepoProgress] = {}
        self._guard = threading.Lock()

    def begin(self, repo_id: str, phase: str) -> None:
        with self._guard:
            self._active[repo_id] = RepoProgress(phase=phase)

    def set_phase(self, repo_id: str, phase: str) -> None:
        """Move to a new phase, resetting the processed/total counters."""
        with self._guard:
            self._active[repo_id] = RepoProgress(phase=phase)

    def set_total(self, repo_id: str, total: int) -> None:
        with self._guard:
            p = self._active.get(repo_id)
            if p is not None:
                p.total = total

    def advance(self, repo_id: str, n: int = 1) -> None:
        with self._guard:
            p = self._active.get(repo_id)
            if p is not None:
                p.processed += n

    def clear(self, repo_id: str) -> None:
        with self._guard:
            self._active.pop(repo_id, None)

    def snapshot(self, repo_id: str) -> RepoProgress | None:
        with self._guard:
            p = self._active.get(repo_id)
            return replace(p) if p is not None else None
