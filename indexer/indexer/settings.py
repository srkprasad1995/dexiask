"""
Runtime configuration for the indexer service.

All values are read from the environment with the ``DEXIASK_`` prefix
(e.g. ``DEXIASK_QDRANT_URL``). Provider API keys are read from the environment
only and are never persisted.
"""
from __future__ import annotations

from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="DEXIASK_",
        env_file=".env",
        extra="ignore",
    )

    # --- HTTP server ---
    server_host: str = "0.0.0.0"
    server_port: int = 8080

    # --- Filesystem ---
    # Host path mounted at /workspace inside the container; repos live beneath it.
    workspace_root: str = "/workspace"
    # Where the service keeps its bare mirrors, doc caches, and repo registry.
    data_dir: str = "/var/lib/dexiask-indexer"
    # Where the declared repo registry yaml lives.
    repos_config_path: str = "repos.yaml"

    # --- Stores ---
    qdrant_url: str = "http://localhost:6333"

    # --- Embeddings ---
    # One of: "auto", "voyage", "openai", "ollama", "hash". Must be identical
    # across repos so their vectors live in a comparable space for cross-repo
    # search. "auto" picks the first configured provider: voyage (if key) →
    # openai (if key) → ollama (if base URL, i.e. the local sidecar is up).
    embedding_provider: str = "auto"
    embedding_model: str = "voyage-code-3"
    # Vector dimension; pinned and stored in each collection's metadata.
    embedding_dim: int = 1024
    embedding_batch_size: int = 128
    voyage_api_key: str = ""
    openai_api_key: str = ""
    # Local Ollama sidecar (no API key). Set by the compose `local` profile;
    # empty = unavailable. The model has its own setting because
    # ``embedding_model`` stays pinned to the hosted default.
    ollama_base_url: str = ""
    ollama_embedding_model: str = "qwen3-embedding:0.6b"

    # --- Remote repo auth ---
    # Default token used to clone/fetch private HTTPS repos when a repo declares a
    # ``url`` and no per-request token is supplied. Read from the env only, never
    # persisted, and never written into a mirror's stored remote URL.
    git_token: str = ""
    # Basic-auth username paired with the token. The default works for GitHub
    # personal/installation tokens; use "oauth2" for GitLab.
    git_token_username: str = "x-access-token"

    # --- Indexing limits / filtering ---
    max_file_bytes: int = 1_000_000
    # Path fragments pruned everywhere (in addition to per-repo excludes).
    skip_dirs: tuple[str, ...] = (
        ".git", "node_modules", "__pycache__", ".next", ".nuxt",
        "dist", "build", ".build", "vendor", "venv", ".venv",
        ".mypy_cache", ".pytest_cache", ".ruff_cache", "target",
    )

    # --- Query defaults / token control ---
    default_search_limit: int = 20
    default_max_tokens: int = 2000
    default_format: str = "toon"  # "toon" | "json"

    # --- Semantic search shaping ---
    # Default result cap for semantic_search (kept small; pointer-first).
    semantic_search_limit: int = 10
    # Restrict semantic results to recognised source code, dropping config/asset/
    # text noise (.gitignore, .editorconfig, .svg, LICENSE, ...). Override per call.
    semantic_search_code_only: bool = True
    # Absolute cosine-score floor; hits below it are dropped (0 = disabled).
    search_min_score: float = 0.0
    # Lines of code included in each result's snippet.
    search_snippet_lines: int = 6

    # --- Scheduling ---
    # Interval for the background reconcile loop that keeps the index converged
    # with each repo's primary-branch tip.
    reindex_interval_s: int = 300

    # --- High-level docs ---
    enable_llm_overview: bool = False
    anthropic_api_key: str = ""
    overview_model: str = "claude-opus-4-8"

    # --- Domain-knowledge docs (generated + embedded, content_type="doc") ---
    enable_domain_docs: bool = False
    domain_docs_model: str = "claude-opus-4-8"

    # --- Per-user repo access gating ---
    # Shared secret with the backend. When set, the indexer gates reads: it
    # validates the caller's forwarded GitHub token against each repo itself, and
    # only honours an "unrestricted" (admin) assertion carrying this token. Unset
    # → no gating (single-user / no-auth).
    internal_token: str = ""


@lru_cache
def get_settings() -> Settings:
    return Settings()
