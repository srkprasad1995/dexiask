"""
Service context: the shared handle tools use to reach repos, the vector store,
and the embedder.

Read tools operate on the existing bare mirror (no fetch — fetch happens during
indexing). The store and embedder are optional so the service degrades
gracefully: lexical/git/read tools work off the mirror even when Qdrant or the
embedding key is unavailable.
"""
from __future__ import annotations

from .config import IndexerConfig, RepoConfig
from .embedding.base import EmbeddingProvider
from .git import GitRepo
from .git.mirror import Mirror, build_repo_mirror
from .settings import Settings
from .store import QdrantStore


class RepoNotFoundError(KeyError):
    pass


class RepoNotIndexedError(RuntimeError):
    pass


class IndexerContext:
    def __init__(
        self,
        settings: Settings,
        registry: IndexerConfig,
        *,
        store: QdrantStore | None = None,
        embedder: EmbeddingProvider | None = None,
    ) -> None:
        self.settings = settings
        self.registry = registry
        self.store = store
        self.embedder = embedder

    @property
    def default_format(self) -> str:
        return self.settings.default_format

    def repo_config(self, repo_id: str) -> RepoConfig:
        repo = self.registry.get(repo_id)
        if repo is None:
            raise RepoNotFoundError(repo_id)
        return repo

    def mirror_for(self, repo_id: str) -> Mirror:
        # Read-only callers never fetch, so no token is needed here.
        return build_repo_mirror(self.settings, self.repo_config(repo_id))

    def git_repo(self, repo_id: str) -> GitRepo:
        """Return a GitRepo on the existing mirror. Raises if not yet indexed."""
        mirror = self.mirror_for(repo_id)
        if not mirror.exists():
            raise RepoNotIndexedError(
                f"repo {repo_id!r} has no mirror yet — trigger an index first"
            )
        return mirror.repo()

    def primary_branch(self, repo_id: str) -> str:
        return self.repo_config(repo_id).primary_branch
