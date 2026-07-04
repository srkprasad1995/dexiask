"""Code-aware chunking: tree-sitter symbol boundaries with a line-window fallback."""

from .chunker import RawChunk, chunk_blob, detect_language, walk_symbols

__all__ = ["RawChunk", "chunk_blob", "detect_language", "walk_symbols"]
