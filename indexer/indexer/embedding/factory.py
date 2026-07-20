"""Build an embedding provider from settings."""
from __future__ import annotations

import logging

from ..settings import Settings
from .base import EmbeddingProvider

log = logging.getLogger("indexer.embedding")


def build_provider(settings: Settings) -> EmbeddingProvider:
    provider = settings.embedding_provider.lower()
    if provider == "auto":
        provider = _resolve_auto(settings)
        log.info("Embedding provider 'auto' resolved to %r", provider)
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
    if provider == "ollama":
        from .ollama import OllamaProvider

        return OllamaProvider(
            base_url=settings.ollama_base_url,
            model=settings.ollama_embedding_model,
            dim=settings.embedding_dim,
            batch_size=settings.embedding_batch_size,
        )
    if provider == "hash":
        from .hashprovider import HashEmbeddingProvider

        return HashEmbeddingProvider(model=settings.embedding_model, dim=settings.embedding_dim)
    raise ValueError(f"unknown embedding provider {settings.embedding_provider!r}")


def _resolve_auto(settings: Settings) -> str:
    """First configured provider wins: hosted keys beat the local sidecar."""
    if settings.voyage_api_key:
        return "voyage"
    if settings.openai_api_key:
        return "openai"
    if settings.ollama_base_url:
        return "ollama"
    raise ValueError(
        "no embedding provider configured: set DEXIASK_VOYAGE_API_KEY / "
        "DEXIASK_OPENAI_API_KEY, or enable the local Ollama sidecar "
        "(COMPOSE_PROFILES=local)"
    )
