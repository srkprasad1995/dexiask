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
import shutil
from typing import Any

from .config import IndexerConfig, RepoConfig
from .context import RepoNotFoundError
from .docs import (
    build_skeleton,
    generate_domain_docs,
    load_domain_docs,
    save_domain_docs,
    save_skeleton,
)
from .docs.overview import generate_overview
from .embedding.base import EmbeddingProvider
from .git import GitRepo
from .git.mirror import Mirror, build_repo_mirror
from .lock import Lock
from .paths import docs_cache_path, mirror_path
from .pipeline import Pipeline
from .pipeline.progress import PHASE_CLONING, PHASE_DOCS, ProgressStore
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
        progress: ProgressStore | None = None,
    ) -> None:
        self.settings = settings
        self.registry = registry
        self.state = state
        self.lock = lock
        self.store = store
        self.progress = progress or ProgressStore()
        self.pipeline: Pipeline | None = (
            Pipeline(settings, store, embedder, state, self.progress)
            if store and embedder
            else None
        )

    def purge_repo(self, repo: RepoConfig) -> None:
        """Delete every index artifact for a repo — its Qdrant collection, bare
        mirror, doc cache, and in-memory state. Removing the registration is the
        caller's concern; this only reclaims what indexing produced."""
        if self.store is not None:
            try:
                self.store.drop_collection(repo.collection)
            except Exception as e:  # best-effort; a missing collection is fine
                log.warning("dropping collection for %s failed: %s", repo.id, e)
        for path in (
            mirror_path(self.settings.data_dir, repo.id),
            docs_cache_path(self.settings.data_dir, repo.id),
        ):
            shutil.rmtree(path, ignore_errors=True)
        self.state.forget(repo.id)

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

            # Publish live progress for the status endpoints; ``clear`` on the way
            # out (including on failure) so a crashed pass never leaves a stuck phase.
            self.progress.begin(repo.id, PHASE_CLONING)
            try:
                mirror = self._mirror(repo, token)
                mirror.ensure()  # clone/fetch (authenticated for private remotes)
                git = mirror.repo()
                # A repo with no pinned branch follows the source's default (e.g.
                # ``trunk``). Capture what the mirror actually tracks so docs, the
                # reconcile, and every read tool key off the real branch.
                if not repo.primary_branch:
                    repo.primary_branch = git.head_branch()
                # High-level docs need only git, so they refresh even in degraded mode.
                self.progress.set_phase(repo.id, PHASE_DOCS)
                self._refresh_docs(repo, git)

                if self.pipeline is None:
                    return {
                        "repo": repo_id,
                        "status": "mirror_refreshed",
                        "semantic_index": "pending",
                    }

                res = self.pipeline.reconcile(repo, git, full=full)
                return {
                    "repo": repo_id,
                    "status": "indexed",
                    "branch": res.branch,
                    "commit": res.commit,
                    "embedded": res.embedded,
                    "removed": res.removed,
                }
            finally:
                self.progress.clear(repo.id)

    def _refresh_docs(self, repo: RepoConfig, git: GitRepo) -> None:
        if repo.primary_branch not in git.branches():
            return
        branch = repo.primary_branch
        try:
            skel = build_skeleton(self.settings, repo.id, git, branch)
            overview = generate_overview(self.settings, skel)
            if overview:
                skel["overview"] = overview
            save_skeleton(self.settings.data_dir, repo.id, branch, skel)
            self._refresh_domain_docs(repo, git, skel, branch)
        except Exception as e:  # docs are best-effort; never fail an index over them
            log.warning("doc refresh for %s failed: %s", repo.id, e)

    def _refresh_domain_docs(self, repo, git, skeleton, branch) -> None:
        """Generate + embed domain-knowledge docs, but only when enabled and the
        branch tip changed since the last run (LLM generation is expensive)."""
        if not self.settings.enable_domain_docs:
            return
        tip = git.resolve(branch)
        cached = load_domain_docs(self.settings.data_dir, repo.id, branch)
        unchanged = cached and tip == self.state.get_commit(repo.id)
        if unchanged:
            return
        docs = generate_domain_docs(self.settings, skeleton)
        save_domain_docs(
            self.settings.data_dir,
            repo.id,
            branch,
            [{"title": d.title, "category": d.category, "slug": d.slug, "body": d.body} for d in docs],
        )
        if self.pipeline is not None:
            self.pipeline.embed_docs(repo, docs)
