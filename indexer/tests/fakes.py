"""Test doubles: a deterministic, network-free embedding provider."""
from __future__ import annotations

import hashlib


class FakeProvider:
    """Hashing bag-of-tokens embedder — deterministic, no network."""

    def __init__(self, model: str = "fake", dim: int = 16) -> None:
        self.model = model
        self.dim = dim

    def _vec(self, text: str) -> list[float]:
        v = [0.0] * self.dim
        for tok in text.split() or [text]:
            h = int(hashlib.blake2b(tok.encode(), digest_size=4).hexdigest(), 16)
            v[h % self.dim] += 1.0
        norm = sum(x * x for x in v) ** 0.5 or 1.0
        return [x / norm for x in v]

    def embed_documents(self, texts: list[str]) -> list[list[float]]:
        return [self._vec(t) for t in texts]

    def embed_query(self, text: str) -> list[float]:
        return self._vec(text)
