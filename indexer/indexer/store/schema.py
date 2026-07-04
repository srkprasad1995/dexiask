"""
Qdrant point schema for code chunks.

One point per ``(repo, path, ordinal)`` on the repo's primary branch. The payload
carries the chunk's single ``path`` and its line range directly (single-tenant,
default-branch only — no branch-ref bookkeeping). Point ids are deterministic so
re-indexing an unchanged tree upserts the same points (idempotent).
"""
from __future__ import annotations

import uuid
from dataclasses import dataclass
from typing import Any

from qdrant_client import models as qm

# Stable namespace so point ids are deterministic across runs (idempotent upsert).
_NAMESPACE = uuid.UUID("c0c0bead-0000-4000-8000-000000000000")


@dataclass(frozen=True)
class Chunk:
    """A code chunk extracted from a single blob."""

    blob_sha: str
    ordinal: int
    text: str
    lang: str
    symbol: str | None = None
    symbol_kind: str | None = None
    start_line: int = 1
    end_line: int = 1


# Virtual-path prefix for generated domain-knowledge docs. Chosen so it can never
# collide with a real repo file path (delete_by_path GC keys off `path`).
DOC_PATH_PREFIX = ".dexiask-docs/"

# Content-type payload values distinguishing embedded source code from generated
# domain-knowledge docs.
CONTENT_CODE = "code"
CONTENT_DOC = "doc"


def point_id(repo_id: str, path: str, ordinal: int) -> str:
    """Deterministic point id → idempotent upserts."""
    return str(uuid.uuid5(_NAMESPACE, f"{repo_id}:{path}:{ordinal}"))


def build_payload(repo_id: str, chunk: Chunk, path: str) -> dict[str, Any]:
    """Build the payload for a freshly-embedded code chunk."""
    return {
        "repo_id": repo_id,
        "content_type": CONTENT_CODE,
        "blob_sha": chunk.blob_sha,
        "ordinal": chunk.ordinal,
        "path": path,
        "lang": chunk.lang,
        "symbol": chunk.symbol,
        "symbol_kind": chunk.symbol_kind,
        "text": chunk.text,
        "start_line": chunk.start_line,
        "end_line": chunk.end_line,
    }


def build_point(repo_id: str, chunk: Chunk, vector: list[float], path: str) -> qm.PointStruct:
    """Construct a Qdrant point for a freshly-embedded code chunk."""
    return qm.PointStruct(
        id=point_id(repo_id, path, chunk.ordinal),
        vector=vector,
        payload=build_payload(repo_id, chunk, path),
    )


def doc_virtual_path(slug: str) -> str:
    """Virtual `path` for a generated domain doc (never a real repo file)."""
    return f"{DOC_PATH_PREFIX}{slug}"


def build_doc_payload(
    repo_id: str, *, path: str, title: str, category: str, ordinal: int, text: str
) -> dict[str, Any]:
    """Build the payload for a generated domain-knowledge doc chunk."""
    return {
        "repo_id": repo_id,
        "content_type": CONTENT_DOC,
        "ordinal": ordinal,
        "path": path,
        "title": title,
        "category": category,
        "lang": "",
        "text": text,
    }


def build_doc_point(
    repo_id: str, *, path: str, title: str, category: str, ordinal: int, text: str, vector: list[float]
) -> qm.PointStruct:
    """Construct a Qdrant point for a generated domain-knowledge doc chunk."""
    return qm.PointStruct(
        id=point_id(repo_id, path, ordinal),
        vector=vector,
        payload=build_doc_payload(
            repo_id, path=path, title=title, category=category, ordinal=ordinal, text=text
        ),
    )
