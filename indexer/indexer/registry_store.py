"""
Persistence for runtime-registered repos.

``repos.yaml`` is the declared, version-controlled base registry. Repos added at
runtime via ``POST /v1/repos`` are persisted here and overlaid on the yaml at
startup, so registrations survive restarts. There is no database: the store is a
single JSON file on the shared filesystem (``<data_dir>/repos.json``).
RepoConfig carries no secrets (auth tokens are per-request / from env), so
storing the full config is safe.
"""
from __future__ import annotations

import json
from pathlib import Path
from typing import Protocol

from .config.models import RepoConfig


class RepoStore(Protocol):
    def list(self) -> list[RepoConfig]: ...

    def upsert(self, repo: RepoConfig) -> None: ...

    def delete(self, repo_id: str) -> bool: ...


class InMemoryRepoStore(RepoStore):
    """Registrations live only for the process (tests / ephemeral deployments)."""

    def __init__(self) -> None:
        self._repos: dict[str, RepoConfig] = {}

    def list(self) -> list[RepoConfig]:
        return list(self._repos.values())

    def upsert(self, repo: RepoConfig) -> None:
        self._repos[repo.id] = repo

    def delete(self, repo_id: str) -> bool:
        return self._repos.pop(repo_id, None) is not None


class FsRepoStore(RepoStore):
    """JSON-file-backed registry on the shared filesystem (no database)."""

    def __init__(self, path: str | Path) -> None:
        self._path = Path(path)

    def _load(self) -> dict[str, RepoConfig]:
        if not self._path.exists():
            return {}
        raw = json.loads(self._path.read_text(encoding="utf-8"))
        return {r["id"]: RepoConfig.model_validate(r) for r in raw}

    def _save(self, repos: dict[str, RepoConfig]) -> None:
        self._path.parent.mkdir(parents=True, exist_ok=True)
        payload = [r.model_dump(mode="json") for r in repos.values()]
        self._path.write_text(json.dumps(payload, indent=2), encoding="utf-8")

    def list(self) -> list[RepoConfig]:
        return list(self._load().values())

    def upsert(self, repo: RepoConfig) -> None:
        repos = self._load()
        repos[repo.id] = repo
        self._save(repos)

    def delete(self, repo_id: str) -> bool:
        repos = self._load()
        if repo_id not in repos:
            return False
        del repos[repo_id]
        self._save(repos)
        return True
