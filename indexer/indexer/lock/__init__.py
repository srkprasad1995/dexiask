"""
Per-repo lock.

A repo's reindex runs under this lock so a manual ``/reindex`` and the background
scheduler never mutate the same repo's collection at once. The service is
single-worker, so a process-local lock is sufficient. ``acquire`` is a context
manager yielding True/False; callers skip work on False.
"""
from __future__ import annotations

import threading
from collections.abc import Iterator
from contextlib import contextmanager
from typing import Protocol


class Lock(Protocol):
    @contextmanager
    def acquire(self, name: str) -> Iterator[bool]: ...


class InMemoryLock(Lock):
    def __init__(self) -> None:
        self._held: set[str] = set()
        self._guard = threading.Lock()

    @contextmanager
    def acquire(self, name: str) -> Iterator[bool]:
        with self._guard:
            if name in self._held:
                got = False
            else:
                self._held.add(name)
                got = True
        try:
            yield got
        finally:
            if got:
                with self._guard:
                    self._held.discard(name)


__all__ = ["InMemoryLock", "Lock"]
