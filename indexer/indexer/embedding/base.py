"""
Embedding provider interface.

Providers are hosted, code-specialized embedding APIs. The interface is
deliberately tiny so swapping providers (or injecting a fake in tests) is
trivial. ``model`` and ``dim`` are surfaced because they are pinned into each
Qdrant collection's metadata and must match across repos for cross-repo search.
"""
from __future__ import annotations

from typing import Protocol, runtime_checkable


@runtime_checkable
class EmbeddingProvider(Protocol):
    model: str
    dim: int

    def embed_documents(self, texts: list[str]) -> list[list[float]]:
        """Embed code chunks for storage. Order matches the input."""
        ...

    def embed_query(self, text: str) -> list[float]:
        """Embed a search query."""
        ...
