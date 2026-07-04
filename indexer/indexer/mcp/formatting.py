"""
Output formatting + token control for MCP tool results.

Every tool can return its payload as **TOON** (Token-Oriented Object Notation,
a compact tabular encoding) or **JSON**, selected by a per-request ``format``
flag. List results are budget-truncated to fit the smaller of ``limit`` and a
``max_tokens`` estimate, and always carry a ``total`` / ``truncated`` marker so
the agent knows when more exists.

TOON is implemented in-repo (rather than via a third-party lib) because our
outputs are uniform record lists, the case TOON encodes most efficiently: a
single header row of field names followed by one comma-delimited row per record.
"""
from __future__ import annotations

import json
from typing import Any

_CHARS_PER_TOKEN = 4


def estimate_tokens(text: str) -> int:
    """Cheap, dependency-free token estimate (~4 chars/token)."""
    return (len(text) + _CHARS_PER_TOKEN - 1) // _CHARS_PER_TOKEN


# ---------------------------------------------------------------------------
# TOON encoder
# ---------------------------------------------------------------------------

def _needs_quote(s: str) -> bool:
    if s == "":
        return True
    if s[0].isspace() or s[-1].isspace():
        return True
    return any(c in s for c in (",", '"', "\n", ":", "[", "]", "{", "}"))


def _scalar(v: Any) -> str:
    if v is None:
        return "null"
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return str(v)
    s = str(v)
    if _needs_quote(s):
        return '"' + s.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n") + '"'
    return s


def _is_uniform_object_list(v: Any) -> bool:
    if not isinstance(v, list) or not v:
        return False
    if not all(isinstance(e, dict) for e in v):
        return False
    keys = list(v[0].keys())
    return all(list(e.keys()) == keys for e in v) and all(
        not isinstance(val, (dict, list)) for e in v for val in e.values()
    )


def encode_toon(data: Any, indent: int = 0) -> str:
    """Encode a JSON-shaped value as TOON. Lossless for our envelope shapes."""
    pad = "  " * indent
    lines: list[str] = []

    if isinstance(data, dict):
        for key, val in data.items():
            if _is_uniform_object_list(val):
                fields = list(val[0].keys())
                lines.append(f"{pad}{key}[{len(val)}]{{{','.join(fields)}}}:")
                for row in val:
                    cells = ",".join(_scalar(row[f]) for f in fields)
                    lines.append(f"{pad}  {cells}")
            elif isinstance(val, list) and all(not isinstance(e, (dict, list)) for e in val):
                cells = ",".join(_scalar(e) for e in val)
                lines.append(f"{pad}{key}[{len(val)}]: {cells}")
            elif isinstance(val, (dict, list)):
                lines.append(f"{pad}{key}:")
                lines.append(encode_toon(val, indent + 1))
            else:
                lines.append(f"{pad}{key}: {_scalar(val)}")
        return "\n".join(lines)

    if isinstance(data, list):
        if _is_uniform_object_list(data):
            fields = list(data[0].keys())
            lines.append(f"{pad}[{len(data)}]{{{','.join(fields)}}}:")
            for row in data:
                lines.append(f"{pad}  " + ",".join(_scalar(row[f]) for f in fields))
            return "\n".join(lines)
        for e in data:
            if isinstance(e, (dict, list)):
                lines.append(encode_toon(e, indent))
            else:
                lines.append(f"{pad}- {_scalar(e)}")
        return "\n".join(lines)

    return f"{pad}{_scalar(data)}"


def encode(data: Any, fmt: str) -> str:
    """Encode ``data`` as TOON or JSON."""
    if fmt == "json":
        return json.dumps(data, indent=2, ensure_ascii=False)
    return encode_toon(data)


# ---------------------------------------------------------------------------
# Token-budgeted row results
# ---------------------------------------------------------------------------

def fit_rows(
    rows: list[dict[str, Any]], *, limit: int, max_tokens: int
) -> tuple[list[dict[str, Any]], bool]:
    """
    Keep the leading rows that fit within ``limit`` and ``max_tokens``.

    Always keeps at least one row (so a single oversized hit is never silently
    dropped). Returns ``(kept_rows, truncated)``.
    """
    kept: list[dict[str, Any]] = []
    spent = 0
    for row in rows[:limit]:
        cost = estimate_tokens(",".join(str(v) for v in row.values()))
        if kept and spent + cost > max_tokens:
            break
        kept.append(row)
        spent += cost
    return kept, len(kept) < len(rows)


def render_results(
    rows: list[dict[str, Any]],
    *,
    fmt: str,
    limit: int,
    max_tokens: int,
    extra: dict[str, Any] | None = None,
) -> str:
    """Build a budget-truncated list envelope and encode it."""
    kept, truncated = fit_rows(rows, limit=limit, max_tokens=max_tokens)
    envelope: dict[str, Any] = {
        "results": kept,
        "total": len(rows),
        "returned": len(kept),
        "truncated": truncated,
    }
    if extra:
        envelope.update(extra)
    return encode(envelope, fmt)


def render_obj(obj: Any, fmt: str) -> str:
    """Encode an arbitrary object (non-list result) as TOON or JSON."""
    return encode(obj, fmt)
