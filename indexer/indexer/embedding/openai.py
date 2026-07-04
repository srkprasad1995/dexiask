"""OpenAI embedding provider (alternative to Voyage)."""
from __future__ import annotations

from ._batching import prepare_batches
from .base import EmbeddingProvider

# text-embedding-3-* caps: 8191 tokens per input, ~300K tokens per request.
_MAX_BATCH_TOKENS = 250_000
_MAX_INPUT_CHARS = 30_000  # ~7.5K tokens, under the 8191 per-input limit


class OpenAIProvider(EmbeddingProvider):
    """Embeds via OpenAI, batching inputs under the per-request and per-input
    limits (a large file or oversized chunk can't overflow a single request)."""

    def __init__(
        self,
        api_key: str,
        model: str = "text-embedding-3-large",
        dim: int = 1024,
        batch_size: int = 128,
        max_batch_tokens: int = _MAX_BATCH_TOKENS,
        max_input_chars: int = _MAX_INPUT_CHARS,
    ) -> None:
        if not api_key:
            raise ValueError("OpenAI API key is required (DEXIASK_OPENAI_API_KEY)")
        from openai import OpenAI

        self.model = model
        self.dim = dim
        self._batch_size = max(1, batch_size)
        self._max_batch_tokens = max_batch_tokens
        self._max_input_chars = max_input_chars
        self._client = OpenAI(api_key=api_key)

    def _embed(self, texts: list[str]) -> list[list[float]]:
        resp = self._client.embeddings.create(input=texts, model=self.model, dimensions=self.dim)
        return [d.embedding for d in resp.data]

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
