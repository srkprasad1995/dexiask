"""
Deterministic, offline embedding provider.

Hashes tokens into a fixed-dimension bag-of-words vector. No network, no API key.
Intended for local development, CI, and demos — it captures lexical overlap, not
real semantics, so it is NOT for production retrieval quality. Selected with
``embedding_provider = "hash"``.
"""
from __future__ import annotations

import hashlib

from .base import EmbeddingProvider


class HashEmbeddingProvider(EmbeddingProvider):
    def __init__(self, model: str = "hash", dim: int = 256) -> None:
        self.model = model
        self.dim = dim

    def _vec(self, text: str) -> list[float]:
        v = [0.0] * self.dim
        for tok in text.split() or [text]:
            h = int(hashlib.blake2b(tok.lower().encode(), digest_size=4).hexdigest(), 16)
            v[h % self.dim] += 1.0
        norm = sum(x * x for x in v) ** 0.5 or 1.0
        return [x / norm for x in v]

    def embed_documents(self, texts: list[str]) -> list[list[float]]:
        return [self._vec(t) for t in texts]

    def embed_query(self, text: str) -> list[float]:
        return self._vec(text)
