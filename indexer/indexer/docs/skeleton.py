"""
Deterministic structural skeleton for a repo at a branch tip.

Built without an LLM: directory map, per-file public symbols (regex per
language), detected entrypoints, README excerpt, and the most-referenced symbols
(a cheap reference-count ranking — the "what matters here" signal, a lightweight
stand-in for a full repo-map PageRank). An optional LLM prose overview can be
layered on top (``overview.py``), off by default.
"""
from __future__ import annotations

import re
from typing import Any

from ..chunking.languages import language_for_path
from ..git import GitRepo
from ..settings import Settings

# Public-symbol patterns per language (line-anchored). Kept simple + dependency-free.
_SYMBOL_RES: dict[str, list[tuple[str, re.Pattern]]] = {
    "python": [
        ("class", re.compile(r"^\s*class\s+(\w+)")),
        ("function", re.compile(r"^\s*(?:async\s+)?def\s+(\w+)")),
    ],
    "go": [
        ("func", re.compile(r"^func\s+(?:\([^)]*\)\s*)?(\w+)")),
        ("type", re.compile(r"^type\s+(\w+)")),
    ],
    "typescript": [
        ("class", re.compile(r"^\s*(?:export\s+)?class\s+(\w+)")),
        ("function", re.compile(r"^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)")),
        ("interface", re.compile(r"^\s*(?:export\s+)?interface\s+(\w+)")),
    ],
    "javascript": [
        ("class", re.compile(r"^\s*(?:export\s+)?class\s+(\w+)")),
        ("function", re.compile(r"^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)")),
    ],
    "rust": [
        ("fn", re.compile(r"^\s*(?:pub\s+)?fn\s+(\w+)")),
        ("struct", re.compile(r"^\s*(?:pub\s+)?struct\s+(\w+)")),
        ("trait", re.compile(r"^\s*(?:pub\s+)?trait\s+(\w+)")),
    ],
    "java": [
        ("class", re.compile(r"^\s*(?:public\s+)?(?:final\s+)?class\s+(\w+)")),
        ("interface", re.compile(r"^\s*(?:public\s+)?interface\s+(\w+)")),
    ],
}

_ENTRYPOINT_HINTS = ("__main__.py", "main.go", "main.py", "main.rs", "index.ts", "index.js")


def _extract_symbols(text: str, lang: str) -> list[tuple[str, str, int]]:
    out: list[tuple[str, str, int]] = []
    rules = _SYMBOL_RES.get(lang)
    if not rules:
        return out
    for lineno, line in enumerate(text.splitlines(), 1):
        for kind, rx in rules:
            m = rx.match(line)
            if m:
                out.append((kind, m.group(1), lineno))
                break
    return out


def build_skeleton(settings: Settings, repo_id: str, git: GitRepo, branch: str) -> dict[str, Any]:
    tip = git.resolve(branch)
    entries = [e for e in git.ls_tree(branch) if e.type == "blob"]

    dirs: set[str] = set()
    files: list[str] = []
    symbols: list[dict[str, Any]] = []
    entrypoints: list[str] = []
    readme = ""
    texts: dict[str, str] = {}

    for e in entries:
        parts = e.path.split("/")
        if any(seg in settings.skip_dirs for seg in parts):
            continue
        files.append(e.path)
        if "/" in e.path:
            dirs.add(e.path.split("/", 1)[0])
        name = parts[-1]
        if name in _ENTRYPOINT_HINTS or parts[0] == "cmd":
            entrypoints.append(e.path)
        if name.lower() == "readme.md" and "/" not in e.path:
            readme = "\n".join(git.read_path(branch, e.path).splitlines()[:30])
        lang = language_for_path(e.path)
        if lang and lang != "markdown":
            try:
                text = git.read_path(branch, e.path)
            except Exception:
                continue
            texts[e.path] = text
            for kind, sym, line in _extract_symbols(text, lang):
                symbols.append({"path": e.path, "kind": kind, "name": sym, "line": line})

    ranked = _rank_symbols(symbols, texts)
    return {
        "repo": repo_id,
        "branch": branch,
        "commit": tip,
        "file_count": len(files),
        "dirs": sorted(dirs),
        "entrypoints": sorted(entrypoints),
        "readme": readme,
        "symbols": symbols,
        "top_symbols": ranked,
    }


def _rank_symbols(
    symbols: list[dict[str, Any]], texts: dict[str, str], top: int = 25
) -> list[dict[str, Any]]:
    """Rank symbols by how many *other* files reference their name (degree centrality)."""
    names = {s["name"] for s in symbols}
    ref_counts: dict[str, int] = dict.fromkeys(names, 0)
    word_res = {n: re.compile(rf"\b{re.escape(n)}\b") for n in names}
    by_def_file: dict[str, set[str]] = {}
    for s in symbols:
        by_def_file.setdefault(s["name"], set()).add(s["path"])
    for path, text in texts.items():
        for name, rx in word_res.items():
            if path in by_def_file.get(name, set()):
                continue
            if rx.search(text):
                ref_counts[name] += 1
    scored = sorted(symbols, key=lambda s: ref_counts.get(s["name"], 0), reverse=True)
    seen: set[tuple[str, str]] = set()
    out: list[dict[str, Any]] = []
    for s in scored:
        key = (s["name"], s["path"])
        if key in seen:
            continue
        seen.add(key)
        out.append({**s, "refs": ref_counts.get(s["name"], 0)})
        if len(out) >= top:
            break
    return out
