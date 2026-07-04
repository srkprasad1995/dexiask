"""Persist high-level docs as JSON on disk (one file per repo+branch)."""
from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from ..paths import docs_cache_path


def _path(data_dir: str, repo_id: str, branch: str) -> Path:
    safe_branch = branch.replace("/", "_")
    return docs_cache_path(data_dir, repo_id) / f"{safe_branch}.json"


def save_skeleton(data_dir: str, repo_id: str, branch: str, skeleton: dict[str, Any]) -> None:
    p = _path(data_dir, repo_id, branch)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps(skeleton), encoding="utf-8")


def load_skeleton(data_dir: str, repo_id: str, branch: str) -> dict[str, Any] | None:
    p = _path(data_dir, repo_id, branch)
    if not p.exists():
        return None
    return json.loads(p.read_text(encoding="utf-8"))
