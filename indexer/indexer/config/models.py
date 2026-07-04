"""
Typed config for the repos the indexer tracks.

A "repo" is a configured folder path (a monorepo is one repo, optionally
``subpath``-scoped) or a git URL. The indexer keeps a bare mirror per repo and
indexes its primary branch. Repo ids are globally unique (single-tenant).
"""
from __future__ import annotations

import re

from pydantic import BaseModel, Field, field_validator, model_validator

# Repo ids become Qdrant collection-name suffixes and tool arguments, so keep
# them to characters safe in both.
_ID_RE = re.compile(r"^[a-zA-Z0-9_-]+$")


class EmbeddingConfig(BaseModel):
    """Per-repo embedding override; falls back to the service default."""

    provider: str = "voyage"
    model: str = "voyage-code-3"
    dim: int = 1024


class RepoConfig(BaseModel):
    """A single tracked repository."""

    id: str
    # Source — exactly one of:
    #  - ``url``: a git remote (https/ssh/git/file) cloned + fetched directly.
    #    Private HTTPS remotes authenticate with a token (per-request or the
    #    ``DEXIASK_GIT_TOKEN`` default); the token is never persisted.
    #  - ``path``: a local checkout under ``workspace_root`` (mirrored from its .git).
    url: str | None = None
    # Path to the repo, relative to ``workspace_root`` (or absolute within it).
    path: str = ""
    # The single branch indexed and searched. Default-branch only by design.
    primary_branch: str = "main"
    # File globs (relative to the repo root / subpath) to include and to exclude.
    # "**" matches any path (including top-level files).
    include: list[str] = Field(default_factory=lambda: ["**"])
    exclude: list[str] = Field(default_factory=list)
    # Monorepo scoping: index only this subtree when set.
    subpath: str | None = None
    # Optional per-repo embedding override.
    embedding: EmbeddingConfig | None = None

    @field_validator("id")
    @classmethod
    def _valid_id(cls, v: str) -> str:
        if not _ID_RE.match(v):
            raise ValueError("repo id may contain only letters, digits, '-' and '_'")
        return v

    @model_validator(mode="after")
    def _require_source(self) -> RepoConfig:
        if not self.url and not self.path:
            raise ValueError(
                "repo requires either 'url' (a git remote) or 'path' (a local checkout)"
            )
        return self

    @property
    def collection(self) -> str:
        """Qdrant collection holding this repo's code chunks."""
        return f"chunks_{self.id}"


class IndexerConfig(BaseModel):
    """The full set of tracked repos plus the default embedding config."""

    repos: list[RepoConfig] = Field(default_factory=list)
    default_embedding: EmbeddingConfig = Field(default_factory=EmbeddingConfig)

    def get(self, repo_id: str) -> RepoConfig | None:
        for r in self.repos:
            if r.id == repo_id:
                return r
        return None

    def embedding_for(self, repo: RepoConfig) -> EmbeddingConfig:
        return repo.embedding or self.default_embedding
