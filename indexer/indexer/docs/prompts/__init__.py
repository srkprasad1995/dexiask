"""LLM prompt templates for doc generation, kept as .md files (never inline)."""
from __future__ import annotations

from functools import lru_cache
from pathlib import Path

_DIR = Path(__file__).parent


@lru_cache(maxsize=8)
def load_prompt(name: str) -> str:
    """Load a prompt template (without the .md extension)."""
    return (_DIR / f"{name}.md").read_text(encoding="utf-8")
