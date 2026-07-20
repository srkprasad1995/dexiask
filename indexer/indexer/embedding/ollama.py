"""Ollama embedding provider (local, no API key; default: qwen3-embedding:0.6b).

Backs the no-API-key local mode: the docker-compose ``local`` profile runs an
Ollama sidecar with the embedding model baked into the image, and the ``auto``
provider falls back here when no hosted-provider key is configured.
"""
from __future__ import annotations

import logging
import time

import httpx

from ._batching import prepare_batches
from .base import EmbeddingProvider

log = logging.getLogger("indexer.embedding")

# Ollama has no per-request input caps like the hosted APIs, but batches are
# processed synchronously — keep them modest so one request can't stall an
# index pass, and truncate inputs well under the model's context window.
_MAX_BATCH_TOKENS = 50_000
_MAX_INPUT_CHARS = 30_000  # ~7.5K tokens, safely under qwen3-embedding's 32K context


class OllamaProvider(EmbeddingProvider):
    """Embeds via a local Ollama server's native ``/api/embed`` endpoint.

    The constructor never touches the network (the sidecar may still be
    starting when the indexer boots); each batch retries with exponential
    backoff, which also absorbs that startup race. The returned vector size is
    validated against the pinned collection dimension so a misconfigured model
    fails loudly instead of writing vectors of the wrong shape.
    """

    def __init__(
        self,
        base_url: str,
        model: str = "qwen3-embedding:0.6b",
        dim: int = 1024,
        max_retries: int = 4,
        sleep=time.sleep,
        batch_size: int = 128,
        max_batch_tokens: int = _MAX_BATCH_TOKENS,
        max_input_chars: int = _MAX_INPUT_CHARS,
        timeout_s: float = 120.0,
    ) -> None:
        if not base_url:
            raise ValueError("Ollama base URL is required (DEXIASK_OLLAMA_BASE_URL)")
        self.model = model
        self.dim = dim
        self._base_url = base_url.rstrip("/")
        self._max_retries = max_retries
        self._sleep = sleep
        self._batch_size = max(1, batch_size)
        self._max_batch_tokens = max_batch_tokens
        self._max_input_chars = max_input_chars
        self._timeout_s = timeout_s

    def _embed(self, texts: list[str]) -> list[list[float]]:
        last: Exception | None = None
        for attempt in range(self._max_retries):
            try:
                resp = httpx.post(
                    f"{self._base_url}/api/embed",
                    json={"model": self.model, "input": texts, "truncate": True},
                    timeout=self._timeout_s,
                )
                resp.raise_for_status()
                embeddings = resp.json()["embeddings"]
                if embeddings and len(embeddings[0]) != self.dim:
                    raise ValueError(
                        f"Ollama model {self.model!r} returned {len(embeddings[0])}-dim "
                        f"vectors but the collection is pinned to {self.dim} "
                        "(DEXIASK_EMBEDDING_DIM); pick a matching model or dim"
                    )
                return embeddings
            except ValueError:
                raise  # dim mismatch is a config error, not transient
            except Exception as e:  # noqa: BLE001 - retry any transient provider error
                last = e
                if attempt < self._max_retries - 1:
                    delay = 4 * (2**attempt)
                    log.warning("Ollama embed failed (attempt %d), retrying in %ds: %s",
                                attempt + 1, delay, e)
                    self._sleep(delay)
        raise last  # type: ignore[misc]

    def embed_documents(self, texts: list[str]) -> list[list[float]]:
        if not texts:
            return []
        out: list[list[float]] = []
        for batch in prepare_batches(
            texts,
            max_items=self._batch_size,
            max_tokens=self._max_batch_tokens,
            max_chars=self._max_input_chars,
        ):
            out.extend(self._embed(batch))
        return out

    def embed_query(self, text: str) -> list[float]:
        return self._embed([text[: self._max_input_chars]])[0]
