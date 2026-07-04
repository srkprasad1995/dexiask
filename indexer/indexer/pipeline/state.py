"""
Write-side bookkeeping.

Two pieces of state make indexing incremental and convergent:
  - ``commit``: the primary-branch commit the repo was last indexed at.
  - ``indexed_paths``: the ``path -> blob_sha`` map of what is currently indexed,
    so an unchanged path (same content sha) is skipped on the next pass.

There is no database: the process-local :class:`InMemoryStateStore` is the only
implementation. Re-indexing on startup rebuilds it, and because point ids are
deterministic the re-index is idempotent.
"""
from __future__ import annotations

from typing import Protocol


class StateStore(Protocol):
    def get_commit(self, repo_id: str) -> str | None: ...
    def set_commit(self, repo_id: str, commit: str) -> None: ...
    def indexed_paths(self, repo_id: str) -> dict[str, str]: ...
    def set_path(self, repo_id: str, path: str, blob_sha: str) -> None: ...
    def remove_path(self, repo_id: str, path: str) -> None: ...


class InMemoryStateStore(StateStore):
    def __init__(self) -> None:
        self._commits: dict[str, str] = {}
        self._paths: dict[str, dict[str, str]] = {}

    def get_commit(self, repo_id: str) -> str | None:
        return self._commits.get(repo_id)

    def set_commit(self, repo_id: str, commit: str) -> None:
        self._commits[repo_id] = commit

    def indexed_paths(self, repo_id: str) -> dict[str, str]:
        return dict(self._paths.get(repo_id, {}))

    def set_path(self, repo_id: str, path: str, blob_sha: str) -> None:
        self._paths.setdefault(repo_id, {})[path] = blob_sha

    def remove_path(self, repo_id: str, path: str) -> None:
        self._paths.get(repo_id, {}).pop(path, None)
