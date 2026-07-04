"""High-level docs: deterministic structural skeleton + optional LLM overview
and generated domain-knowledge docs."""

from .domain import DomainDoc, generate_domain_docs
from .skeleton import build_skeleton
from .store import (
    load_domain_docs,
    load_skeleton,
    save_domain_docs,
    save_skeleton,
)

__all__ = [
    "DomainDoc",
    "build_skeleton",
    "generate_domain_docs",
    "load_domain_docs",
    "load_skeleton",
    "save_domain_docs",
    "save_skeleton",
]
