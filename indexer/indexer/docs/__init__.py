"""High-level docs: deterministic structural skeleton + optional LLM overview."""

from .skeleton import build_skeleton
from .store import load_skeleton, save_skeleton

__all__ = ["build_skeleton", "load_skeleton", "save_skeleton"]
