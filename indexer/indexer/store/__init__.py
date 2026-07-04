"""Qdrant collection + payload schema and the store client."""

from .qdrant import QdrantStore, SearchHit
from .schema import (
    CONTENT_CODE,
    CONTENT_DOC,
    DOC_PATH_PREFIX,
    Chunk,
    build_doc_point,
    build_point,
    doc_virtual_path,
    point_id,
)

__all__ = [
    "CONTENT_CODE",
    "CONTENT_DOC",
    "DOC_PATH_PREFIX",
    "Chunk",
    "QdrantStore",
    "SearchHit",
    "build_doc_point",
    "build_point",
    "doc_virtual_path",
    "point_id",
]
