"""
Index orchestration.

``IndexService.index_repo`` is the single entry every trigger (scheduler, manual)
funnels through. It runs under the per-repo lock so only one caller mutates a
repo at a time, refreshes the bare mirror, updates the high-level docs, then
reconciles the primary branch (idempotent, convergent).

When no store/embedder is configured it still refreshes the mirror (so
lexical/git/read tools serve live data) and reports the semantic index as pending.
"""
from __future__ import annotations

import logging
from typing import Any

from .config import IndexerConfig, RepoConfig
from .context import RepoNotFoundError
from .docs import build_skeleton, save_skeleton
from .docs.overview import generate_overview
from .embedding.base import EmbeddingProvider
from .git import GitRepo
from .git.mirror import Mirror, build_repo_mirror
from .lock import Lock
from .pipeline import Pipeline
from .pipeline.state import StateStore
from .settings import Settings
from .store import QdrantStore

log = logging.getLogger("indexer.service")


class IndexService:
    def __init__(
        self,
        settings: Settings,
        registry: IndexerConfig,
        *,
        store: QdrantStore | None,
        embedder: EmbeddingProvider | None,
        state: StateStore,
        lock: Lock,
    ) -> None:
        self.settings = settings
        self.registry = registry
        self.state = state
        self.lock = lock
        self.pipeline: Pipeline | None = (
            Pipeline(settings, store, embedder, state) if store and embedder else None
        )

    def _mirror(self, repo: RepoConfig, token: str | None = None) -> Mirror:
        return build_repo_mirror(self.settings, repo, token=token)

    def index_repo(
        self, repo_id: str, *, full: bool = False, token: str | None = None
    ) -> dict[str, Any]:
        repo = self.registry.get(repo_id)
        if repo is None:
            raise RepoNotFoundError(repo_id)

        with self.lock.acquire(f"index:{repo.id}") as got:
            if not got:
                return {"repo": repo_id, "status": "already_running"}

            mirror = self._mirror(repo, token)
            mirror.ensure()  # clone/fetch (authenticated for private remotes)
            git = mirror.repo()
            # High-level docs need only git, so they refresh even in degraded mode.
            self._refresh_docs(repo, git)

            if self.pipeline is None:
                return {"repo": repo_id, "status": "mirror_refreshed", "semantic_index": "pending"}

            res = self.pipeline.reconcile(repo, git, full=full)
            return {
                "repo": repo_id,
                "status": "indexed",
                "branch": res.branch,
                "commit": res.commit,
                "embedded": res.embedded,
                "removed": res.removed,
            }

    def _refresh_docs(self, repo: RepoConfig, git: GitRepo) -> None:
        if repo.primary_branch not in git.branches():
            return
        try:
            skel = build_skeleton(self.settings, repo.id, git, repo.primary_branch)
            overview = generate_overview(self.settings, skel)
            if overview:
                skel["overview"] = overview
            save_skeleton(self.settings.data_dir, repo.id, repo.primary_branch, skel)
        except Exception as e:  # docs are best-effort; never fail an index over them
            log.warning("doc refresh for %s failed: %s", repo.id, e)
