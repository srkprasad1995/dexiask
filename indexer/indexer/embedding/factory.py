"""Build an embedding provider from settings."""
from __future__ import annotations

from ..settings import Settings
from .base import EmbeddingProvider


def build_provider(settings: Settings) -> EmbeddingProvider:
    provider = settings.embedding_provider.lower()
    if provider == "voyage":
        from .voyage import VoyageProvider

        return VoyageProvider(
            api_key=settings.voyage_api_key,
            model=settings.embedding_model,
            dim=settings.embedding_dim,
            batch_size=settings.embedding_batch_size,
        )
    if provider == "openai":
        from .openai import OpenAIProvider

        return OpenAIProvider(
            api_key=settings.openai_api_key,
            model=settings.embedding_model,
            dim=settings.embedding_dim,
            batch_size=settings.embedding_batch_size,
        )
    if provider == "hash":
        from .hashprovider import HashEmbeddingProvider

        return HashEmbeddingProvider(model=settings.embedding_model, dim=settings.embedding_dim)
    raise ValueError(f"unknown embedding provider {settings.embedding_provider!r}")
