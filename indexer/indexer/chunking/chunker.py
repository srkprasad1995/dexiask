"""
Chunk a blob into code units.

Preferred: tree-sitter symbol boundaries (functions/classes/methods), so a chunk
is a coherent unit and a small edit only changes the chunks it touches. Oversized
symbols are split by a sliding line window. When tree-sitter or a grammar is
unavailable, the whole file is chunked by line window. Binary blobs yield nothing.
"""
from __future__ import annotations

from dataclasses import dataclass

from .languages import SYMBOL_NODE_TYPES, language_for_path

_MAX_LINES = 80
_OVERLAP = 15


@dataclass(frozen=True)
class RawChunk:
    text: str
    start_line: int
    end_line: int
    symbol: str | None = None
    symbol_kind: str | None = None


def detect_language(path: str) -> str | None:
    return language_for_path(path)


def _is_binary(content: bytes) -> bool:
    return b"\x00" in content[:8192]


def _line_windows(
    lines: list[str], *, base: int = 1, max_lines: int = _MAX_LINES, overlap: int = _OVERLAP
) -> list[RawChunk]:
    chunks: list[RawChunk] = []
    step = max(1, max_lines - overlap)
    i = 0
    n = len(lines)
    while i < n:
        window = lines[i : i + max_lines]
        chunks.append(
            RawChunk(
                text="\n".join(window),
                start_line=base + i,
                end_line=base + i + len(window) - 1,
            )
        )
        if i + max_lines >= n:
            break
        i += step
    return chunks


def _coarse_kind(node_type: str) -> str:
    t = node_type.lower()
    if "class" in t:
        return "class"
    if "method" in t:
        return "method"
    if "interface" in t:
        return "interface"
    if "struct" in t or "type" in t or "trait" in t or "impl" in t or "enum" in t:
        return "type"
    if "function" in t or "func" in t or "method" in t:
        return "function"
    return "symbol"


def _symbol_name(node) -> str | None:
    for child in node.children:
        if child.type in ("identifier", "name", "field_identifier", "type_identifier"):
            return child.text.decode("utf-8", "replace")
    return None


def walk_symbols(root_node, lang: str, lines: list[str]) -> list[RawChunk]:
    """Build chunks from a parsed tree's top-level symbol nodes.

    Pure (takes an already-parsed node), so it is unit-tested without a live
    grammar. Oversized symbols are split by line window.
    """
    types = SYMBOL_NODE_TYPES.get(lang, set())
    chunks: list[RawChunk] = []
    for node in root_node.children:
        if node.type not in types:
            continue
        start = node.start_point[0] + 1
        end = node.end_point[0] + 1
        kind = _coarse_kind(node.type)
        name = _symbol_name(node)
        if end - start + 1 > _MAX_LINES:
            for w in _line_windows(lines[start - 1 : end], base=start):
                chunks.append(RawChunk(w.text, w.start_line, w.end_line, name, kind))
        else:
            chunks.append(RawChunk("\n".join(lines[start - 1 : end]), start, end, name, kind))
    return chunks


def _try_treesitter(text: str, lang: str) -> list[RawChunk] | None:
    if lang not in SYMBOL_NODE_TYPES:
        return None
    try:  # pragma: no cover - exercised only when a working grammar is installed
        from tree_sitter_language_pack import get_parser

        tree = get_parser(lang).parse(text.encode("utf-8"))
    except Exception:
        return None
    return walk_symbols(tree.root_node, lang, text.splitlines()) or None  # pragma: no cover


def chunk_blob(content: bytes, path: str) -> list[RawChunk]:
    """Return ordered chunks for a blob, or [] for binary/empty content."""
    if not content or _is_binary(content):
        return []
    text = content.decode("utf-8", "replace")
    if not text.strip():
        return []
    lang = detect_language(path)
    if lang:
        ts = _try_treesitter(text, lang)
        if ts is not None:
            return ts
    return _line_windows(text.splitlines())
