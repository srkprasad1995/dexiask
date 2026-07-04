"""Voyage AI embedding provider (default: voyage-code-3)."""
from __future__ import annotations

import logging
import time

from ._batching import prepare_batches
from .base import EmbeddingProvider

log = logging.getLogger("indexer.embedding")

# Voyage per-request caps: up to 1000 inputs and ~120K tokens total, 32K tokens
# per input. We stay conservatively under each.
_MAX_BATCH_TOKENS = 100_000
_MAX_INPUT_CHARS = 120_000  # ~30K tokens, safely under the 32K per-input limit


class VoyageProvider(EmbeddingProvider):
    """Embeds via Voyage AI, using code-aware input types.

    Inputs are truncated per the per-input token cap and packed into batches
    that respect Voyage's per-request input-count and total-token limits, so a
    large file (or a single huge chunk) can't overflow one request. Each batch
    retries with exponential backoff on transient failures (rate limits,
    timeouts) so an index pass survives Voyage's per-minute limits.
    """

    def __init__(
        self,
        api_key: str,
        model: str = "voyage-code-3",
        dim: int = 1024,
        max_retries: int = 4,
        sleep=time.sleep,
        batch_size: int = 128,
        max_batch_tokens: int = _MAX_BATCH_TOKENS,
        max_input_chars: int = _MAX_INPUT_CHARS,
    ) -> None:
        if not api_key:
            raise ValueError("Voyage API key is required (DEXIASK_VOYAGE_API_KEY)")
        import voyageai

        self.model = model
        self.dim = dim
        self._max_retries = max_retries
        self._sleep = sleep
        self._batch_size = max(1, batch_size)
        self._max_batch_tokens = max_batch_tokens
        self._max_input_chars = max_input_chars
        self._client = voyageai.Client(api_key=api_key)

    def _embed(self, texts: list[str], input_type: str) -> list[list[float]]:
        last: Exception | None = None
        for attempt in range(self._max_retries):
            try:
                result = self._client.embed(
                    texts, model=self.model, input_type=input_type, output_dimension=self.dim
                )
                return result.embeddings
            except Exception as e:  # noqa: BLE001 - retry any transient provider error
                last = e
                if attempt < self._max_retries - 1:
                    delay = 4 * (2**attempt)
                    log.warning("Voyage embed failed (attempt %d), retrying in %ds: %s",
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
            out.extend(self._embed(batch, "document"))
        return out

    def embed_query(self, text: str) -> list[float]:
        return self._embed([text[: self._max_input_chars]], "query")[0]
