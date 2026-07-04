"""
Load and merge the repo registry.

The primary source is ``repos.yaml``; repos registered at runtime via the
control-plane API are merged on top (by id). ``load_registry`` is pure (takes raw
text) so it is trivially testable; ``load_registry_file`` reads from disk.
"""
from __future__ import annotations

from pathlib import Path

import yaml

from .models import EmbeddingConfig, IndexerConfig, RepoConfig


def load_registry(yaml_text: str) -> IndexerConfig:
    """Parse registry yaml text into an ``IndexerConfig``."""
    data = yaml.safe_load(yaml_text) or {}
    repos = [RepoConfig.model_validate(r) for r in data.get("repos", [])]
    default = data.get("default_embedding")
    cfg = IndexerConfig(
        repos=repos,
        default_embedding=EmbeddingConfig.model_validate(default) if default else EmbeddingConfig(),
    )
    _ensure_unique_ids(cfg.repos)
    return cfg


def load_registry_file(path: str) -> IndexerConfig:
    p = Path(path)
    if not p.exists():
        return IndexerConfig()
    return load_registry(p.read_text(encoding="utf-8"))


def merge_repo(cfg: IndexerConfig, repo: RepoConfig) -> IndexerConfig:
    """Return a new config with ``repo`` added or replacing an existing one (by id)."""
    repos = [r for r in cfg.repos if r.id != repo.id] + [repo]
    return IndexerConfig(repos=repos, default_embedding=cfg.default_embedding)


def _ensure_unique_ids(repos: list[RepoConfig]) -> None:
    seen: set[str] = set()
    for r in repos:
        if r.id in seen:
            raise ValueError(f"duplicate repo id {r.id!r}")
        seen.add(r.id)
