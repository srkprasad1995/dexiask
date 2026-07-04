"""Shared, LLM-facing digest of a repo skeleton (used by overview + domain docs)."""
from __future__ import annotations

from typing import Any


def skeleton_digest(skeleton: dict[str, Any]) -> dict[str, Any]:
    """Trim a full skeleton to the compact structure fed to the LLM."""
    return {
        "dirs": skeleton.get("dirs"),
        "entrypoints": skeleton.get("entrypoints"),
        "readme": skeleton.get("readme"),
        "top_symbols": [
            {"name": s["name"], "path": s["path"], "kind": s["kind"]}
            for s in skeleton.get("top_symbols", [])
        ],
    }
