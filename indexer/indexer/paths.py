"""
Path resolution and jailing.

Repos live under ``workspace_root`` (the host path mounted at /workspace). All
resolution is jailed to ``workspace_root`` so a malicious ``path`` cannot escape.
The bare mirror for each repo lives under ``<data_dir>/mirrors/<repo_id>.git`` and
its doc cache under ``<data_dir>/docs/<repo_id>``.
"""
from __future__ import annotations

from pathlib import Path


def resolve_repo_path(workspace_root: str, path: str) -> Path:
    """
    Resolve a repo's on-disk location, jailed to ``workspace_root``.

    ``path`` is treated as relative to ``workspace_root``. An absolute ``path``
    already inside the workspace root is honored as-is; otherwise its leading
    slash is dropped and it is re-rooted. Raises ``ValueError`` on a traversal
    attempt.
    """
    root = Path(workspace_root).resolve()
    raw = Path(path)
    if raw.is_absolute():
        resolved = raw.resolve()
        try:
            resolved.relative_to(root)
            candidate = resolved
        except ValueError:
            candidate = (root / Path(*raw.parts[1:])).resolve()
    else:
        candidate = (root / raw).resolve()
    try:
        candidate.relative_to(root)
    except ValueError as e:
        raise ValueError(f"repo path {path!r} escapes workspace root") from e
    return candidate


def mirror_path(data_dir: str, repo_id: str) -> Path:
    return Path(data_dir).resolve() / "mirrors" / f"{repo_id}.git"


def docs_cache_path(data_dir: str, repo_id: str) -> Path:
    return Path(data_dir).resolve() / "docs" / repo_id
