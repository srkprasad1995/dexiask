"""
MCP tool registry: definitions + handlers.

Each tool is ``(name, description, input_schema, handler)``. Handlers are async,
take the ``IndexerContext`` and the raw args, and return an already-formatted
string (TOON or JSON per the request's ``format``). Tool and argument names use
searchable keywords because an MCP router token-scores them during discovery.

Lexical/git/read tools work entirely off the bare mirror. ``semantic_search``
degrades gracefully when no embedder/store is configured. Everything operates on
each repo's primary branch (single-tenant, default-branch only); the git
introspection tools additionally accept explicit revs.
"""
from __future__ import annotations

import fnmatch
import logging
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Any

from ..context import IndexerContext, RepoNotIndexedError
from ..docs import load_skeleton
from . import formatting as fmt

log = logging.getLogger("indexer.tools")

Handler = Callable[[IndexerContext, dict[str, Any]], Awaitable[str]]

# --- shared schema fragments ------------------------------------------------
_FMT = {"type": "string", "enum": ["toon", "json"], "description": "output format (default toon)"}
_LIMIT = {"type": "integer", "description": "max results to return"}
_MAXTOK = {"type": "integer", "description": "token budget for the result set"}
_REPO = {"type": "string", "description": "repo id"}

# Langs that are not source code: the pipeline's "text" fallback for any
# unrecognised file (.gitignore, .editorconfig, .svg, .rst, LICENSE, ...).
_NONCODE_LANGS = {"", "text"}


def _fmt(ctx: IndexerContext, args: dict) -> str:
    return args.get("format") or ctx.default_format


def _limit(ctx: IndexerContext, args: dict) -> int:
    return int(args.get("limit") or ctx.settings.default_search_limit)


def _maxtok(ctx: IndexerContext, args: dict) -> int:
    return int(args.get("max_tokens") or ctx.settings.default_max_tokens)


# --- handlers ---------------------------------------------------------------

async def list_repos(ctx: IndexerContext, args: dict) -> str:
    rows = [
        {"id": r.id, "path": r.path, "url": r.url or "", "primary_branch": r.primary_branch}
        for r in ctx.registry.repos
    ]
    return fmt.render_results(rows, fmt=_fmt(ctx, args), limit=len(rows) or 1, max_tokens=10_000)


async def lexical_search(ctx: IndexerContext, args: dict) -> str:
    repo = ctx.git_repo(args["repo"])
    branch = ctx.primary_branch(args["repo"])
    pathspec = args.get("glob") or args.get("path")
    hits = repo.grep(args["pattern"], branch, pathspec=pathspec, max_hits=_limit(ctx, args) * 5)
    rows = [{"path": h.path, "line": h.lineno, "text": h.line.strip()} for h in hits]
    return fmt.render_results(rows, fmt=_fmt(ctx, args), limit=_limit(ctx, args), max_tokens=_maxtok(ctx, args))


async def glob(ctx: IndexerContext, args: dict) -> str:
    repo = ctx.git_repo(args["repo"])
    branch = ctx.primary_branch(args["repo"])
    pattern = args["pattern"]
    rows = []
    for e in repo.ls_tree(branch):
        if e.type != "blob":
            continue
        if fnmatch.fnmatch(e.path, pattern) or fnmatch.fnmatch(e.path.rsplit("/", 1)[-1], pattern):
            rows.append({"path": e.path})
    return fmt.render_results(rows, fmt=_fmt(ctx, args), limit=_limit(ctx, args), max_tokens=_maxtok(ctx, args))


async def read_range(ctx: IndexerContext, args: dict) -> str:
    repo = ctx.git_repo(args["repo"])
    branch = ctx.primary_branch(args["repo"])
    text = repo.read_path(branch, args["path"])
    lines = text.splitlines()
    offset = max(1, int(args.get("offset") or 1))
    limit = int(args.get("limit") or 200)
    selected = lines[offset - 1 : offset - 1 + limit]
    numbered = "\n".join(f"{offset + i:6d}\t{ln}" for i, ln in enumerate(selected))
    return f"{args['path']}@{branch} (lines {offset}-{offset + len(selected) - 1}):\n{numbered}"


async def explore(ctx: IndexerContext, args: dict) -> str:
    repo = ctx.git_repo(args["repo"])
    branch = ctx.primary_branch(args["repo"])
    prefix = (args.get("path") or "").strip("/")
    prefix_seg = f"{prefix}/" if prefix else ""
    dirs: set[str] = set()
    files: list[str] = []
    for e in repo.ls_tree(branch):
        if e.type != "blob" or not e.path.startswith(prefix_seg):
            continue
        rest = e.path[len(prefix_seg):]
        if "/" in rest:
            dirs.add(rest.split("/", 1)[0] + "/")
        else:
            files.append(rest)
    rows = [{"entry": d, "type": "dir"} for d in sorted(dirs)]
    rows += [{"entry": f, "type": "file"} for f in sorted(files)]
    return fmt.render_results(
        rows, fmt=_fmt(ctx, args), limit=_limit(ctx, args), max_tokens=_maxtok(ctx, args),
        extra={"path": prefix or "/", "branch": branch},
    )


def _search_hits(ctx: IndexerContext, args: dict) -> list[tuple[str, Any]]:
    """Shared semantic-search core for both the MCP ``semantic_search`` tool and
    the ``/v1/search`` REST endpoint.

    Embeds the query, searches each target repo's collection, applies the
    language/score filters, and returns ``(repo_id, SearchHit)`` pairs sorted by
    score (descending). The caller must ensure ``ctx.embedder`` and ``ctx.store``
    are configured.
    """
    # Repos to search: an explicit `repos` list or a single `repo` wins; otherwise
    # search ALL registered repos (global search).
    explicit = bool(args.get("repos") or args.get("repo"))
    if args.get("repos"):
        repo_ids = args["repos"]
    elif args.get("repo"):
        repo_ids = [args["repo"]]
    else:
        repo_ids = [r.id for r in ctx.registry.repos]
    qv = ctx.embedder.embed_query(args["query"])
    limit = int(args.get("limit") or ctx.settings.semantic_search_limit)
    code_only = (
        ctx.settings.semantic_search_code_only
        if args.get("code_only") is None
        else bool(args.get("code_only"))
    )
    min_score = (
        float(args["min_score"]) if args.get("min_score") is not None
        else ctx.settings.search_min_score
    )
    # Over-fetch per repo so score/code filtering doesn't starve the final cap.
    fetch_k = min(max(limit * 4, 40), 200)
    hits: list[tuple[str, Any]] = []
    for rid in repo_ids:
        try:
            repo = ctx.repo_config(rid)
            found = ctx.store.search(
                repo.collection, qv, limit=fetch_k,
                lang=args.get("lang"), path_prefix=args.get("path_prefix"),
                symbol_kind=args.get("symbol_kind"),
            )
        except Exception:
            # Global (all-repos) search: skip repos that aren't searchable yet
            # (not indexed / no collection). An explicitly-named repo still errors.
            if explicit:
                raise
            log.debug("semantic_search: skipping repo %s", rid, exc_info=True)
            continue
        for h in found:
            lang = h.payload.get("lang") or ""
            if code_only and lang in _NONCODE_LANGS:
                continue  # drop config/asset/text noise (the "text" fallback / unknown)
            if min_score > 0 and h.score < min_score:
                continue
            hits.append((rid, h))
    hits.sort(key=lambda rh: rh[1].score, reverse=True)
    return hits


async def semantic_search(ctx: IndexerContext, args: dict) -> str:
    if ctx.embedder is None or ctx.store is None:
        return fmt.render_obj(
            {"error": "semantic search unavailable: no embedder/store configured"},
            _fmt(ctx, args),
        )
    limit = int(args.get("limit") or ctx.settings.semantic_search_limit)
    merged = [
        {
            "repo": rid,
            "path": h.payload.get("path", ""),
            "lines": f"{h.payload.get('start_line', '')}-{h.payload.get('end_line', '')}",
            "lang": h.payload.get("lang") or "",
            "symbol": h.payload.get("symbol") or "",
            "symbol_kind": h.payload.get("symbol_kind") or "",
            "score": round(h.score, 4),
            "code": _snippet(h.payload.get("text") or "", ctx.settings.search_snippet_lines),
        }
        for rid, h in _search_hits(ctx, args)
    ]
    return fmt.render_results(merged, fmt=_fmt(ctx, args), limit=limit, max_tokens=_maxtok(ctx, args))


def search_json(ctx: IndexerContext, args: dict) -> dict[str, Any]:
    """JSON variant of :func:`semantic_search` for the web indexer page.

    Returns ``{"results": [...]}`` with camelCase fields the UI renders directly.
    Degrades to an empty result set when no embedder/store is configured.
    """
    if ctx.embedder is None or ctx.store is None:
        return {"results": [], "error": "semantic search unavailable"}
    limit = int(args.get("limit") or ctx.settings.semantic_search_limit)
    results = [
        {
            "repo": rid,
            "path": h.payload.get("path", ""),
            "startLine": h.payload.get("start_line"),
            "endLine": h.payload.get("end_line"),
            "language": h.payload.get("lang") or "",
            "symbol": h.payload.get("symbol") or "",
            "score": round(h.score, 4),
            "content": h.payload.get("text") or "",
        }
        for rid, h in _search_hits(ctx, args)[:limit]
    ]
    return {"results": results}


async def get_chunk(ctx: IndexerContext, args: dict) -> str:
    if args.get("path"):
        return await read_range(ctx, {**args, "limit": int(args.get("limit") or 400)})
    return fmt.render_obj({"error": "get_chunk needs a path"}, _fmt(ctx, args))


async def get_overview(ctx: IndexerContext, args: dict) -> str:
    repo = ctx.repo_config(args["repo"])
    skel = load_skeleton(ctx.settings.data_dir, repo.id, repo.primary_branch)
    if not skel:
        return fmt.render_obj({"error": f"no docs for {repo.id!r} yet — index it first"}, _fmt(ctx, args))
    summary = {
        k: skel.get(k)
        for k in ("repo", "branch", "commit", "file_count", "dirs", "entrypoints", "readme")
    }
    if skel.get("overview"):
        summary["overview"] = skel["overview"]
    summary["top_symbols"] = skel.get("top_symbols", [])[:15]
    return fmt.render_obj(summary, _fmt(ctx, args))


async def get_docs(ctx: IndexerContext, args: dict) -> str:
    repo = ctx.repo_config(args["repo"])
    skel = load_skeleton(ctx.settings.data_dir, repo.id, repo.primary_branch)
    if not skel:
        return fmt.render_obj({"error": f"no docs for {repo.id!r} yet — index it first"}, _fmt(ctx, args))
    syms = skel.get("symbols", [])
    prefix = args.get("path")
    if prefix:
        syms = [s for s in syms if s["path"].startswith(prefix)]
    return fmt.render_results(
        syms, fmt=_fmt(ctx, args), limit=_limit(ctx, args), max_tokens=_maxtok(ctx, args),
        extra={"path": prefix or "/", "branch": repo.primary_branch},
    )


def _git_text(method: str) -> Handler:
    async def handler(ctx: IndexerContext, args: dict) -> str:
        repo = ctx.git_repo(args["repo"])
        primary = ctx.primary_branch(args["repo"])
        if method == "log":
            branch = args.get("branch") or primary
            rows = repo.log(branch, path=args.get("path"), limit=_limit(ctx, args))
            return fmt.render_results(rows, fmt=_fmt(ctx, args), limit=_limit(ctx, args), max_tokens=_maxtok(ctx, args))
        if method == "branches":
            rows = [{"branch": b} for b in repo.branches()]
            return fmt.render_results(rows, fmt=_fmt(ctx, args), limit=len(rows) or 1, max_tokens=10_000)
        if method == "blame":
            return _cap(repo.blame(args.get("rev") or primary, args["path"]), ctx, args)
        if method == "diff":
            full = repo.diff(args["from_rev"], args["to_rev"], path=args.get("path"))
            budget = max(_maxtok(ctx, args), 200) * 4
            if len(full) <= budget:
                return full
            stat = repo.diff_stat(args["from_rev"], args["to_rev"], path=args.get("path"))
            return _cap(
                f"Diff {args['from_rev']}..{args['to_rev']} is large "
                f"({len(full)} chars) — showing the file summary instead of the full patch:\n\n"
                f"{stat}\n[Pass a 'path' to see one file's full diff, or raise 'max_tokens'.]",
                ctx, args,
            )
        if method == "show":
            return _cap(repo.show(args["rev"]), ctx, args)
        if method == "file_at_rev":
            return _cap(repo.read_path(args["rev"], args["path"]), ctx, args)
        raise ValueError(method)  # pragma: no cover

    return handler


def _cap(text: str, ctx: IndexerContext, args: dict) -> str:
    """Bound a raw git payload (diff/blame/show/file) to the token budget.

    Unlike the row-based tools, these return free-form text; an un-budgeted diff
    can be megabytes and overflow the model's context. Truncate to ~``max_tokens``
    worth of chars and point the agent at narrower options.
    """
    budget = max(_maxtok(ctx, args), 200) * 4  # ~4 chars/token
    if len(text) <= budget:
        return text
    head = text[:budget].rsplit("\n", 1)[0]  # don't cut mid-line
    return (
        f"{head}\n\n[truncated — {len(text)} chars total, showed ~{len(head)}. "
        "Narrow with a 'path' argument, or use semantic_search / read_range for "
        "specific files.]"
    )


def _snippet(text: str, max_lines: int, max_chars: int = 500) -> str:
    """A compact, leading-blank-trimmed code preview of up to ``max_lines``."""
    if not text:
        return ""
    lines = text.splitlines()
    while lines and not lines[0].strip():
        lines.pop(0)
    return "\n".join(lines[:max_lines]).rstrip()[:max_chars]


@dataclass(frozen=True)
class Tool:
    name: str
    description: str
    input_schema: dict[str, Any]
    handler: Handler


def build_tools() -> list[Tool]:
    return [
        Tool("semantic_search",
             "Semantic code search by meaning. Returns ranked hits (repo, path, lines, lang, "
             "symbol, symbol_kind, score, code); use read_range/get_chunk for full source. "
             "Code-only by default (config/asset/text noise is dropped); set code_only=false to "
             "include docs. Supports cross-repo via 'repos'; omit repo/repos for a global search.",
             {"type": "object", "properties": {
                 "query": {"type": "string"}, "repo": _REPO,
                 "repos": {"type": "array", "items": {"type": "string"},
                           "description": "search several repos at once"},
                 "lang": {"type": "string"},
                 "path_prefix": {"type": "string"}, "symbol_kind": {"type": "string"},
                 "code_only": {"type": "boolean",
                               "description": "restrict to source code (default true)"},
                 "min_score": {"type": "number",
                               "description": "drop hits below this cosine score"},
                 "limit": _LIMIT, "max_tokens": _MAXTOK, "format": _FMT,
             }, "required": ["query"]},
             semantic_search),
        Tool("get_chunk",
             "Expand a search hit to its full source. Pass repo + path.",
             {"type": "object", "properties": {
                 "repo": _REPO, "path": {"type": "string"},
                 "symbol": {"type": "string"}, "format": _FMT,
             }, "required": ["repo"]},
             get_chunk),
        Tool("lexical_search",
             "Exact regex search over the repo (git grep). Returns file:line:content.",
             {"type": "object", "properties": {
                 "pattern": {"type": "string"}, "repo": _REPO,
                 "glob": {"type": "string"}, "path": {"type": "string"},
                 "limit": _LIMIT, "format": _FMT,
             }, "required": ["pattern", "repo"]},
             lexical_search),
        Tool("glob",
             "Find files matching a glob pattern.",
             {"type": "object", "properties": {
                 "pattern": {"type": "string"}, "repo": _REPO,
                 "limit": _LIMIT, "format": _FMT,
             }, "required": ["pattern", "repo"]},
             glob),
        Tool("read_range",
             "Read a file (or line range), with line numbers.",
             {"type": "object", "properties": {
                 "path": {"type": "string"}, "repo": _REPO,
                 "offset": {"type": "integer"}, "limit": _LIMIT,
             }, "required": ["path", "repo"]},
             read_range),
        Tool("explore",
             "List directory entries at a path.",
             {"type": "object", "properties": {
                 "repo": _REPO, "path": {"type": "string"},
                 "limit": _LIMIT, "format": _FMT,
             }, "required": ["repo"]},
             explore),
        Tool("git_log", "Commit history for the repo or a path.",
             {"type": "object", "properties": {
                 "repo": _REPO, "branch": {"type": "string"}, "path": {"type": "string"},
                 "limit": _LIMIT, "format": _FMT,
             }, "required": ["repo"]}, _git_text("log")),
        Tool("git_branches", "List branches in the repo.",
             {"type": "object", "properties": {"repo": _REPO, "format": _FMT}, "required": ["repo"]},
             _git_text("branches")),
        Tool("git_blame", "Line-by-line blame for a file at a rev.",
             {"type": "object", "properties": {
                 "repo": _REPO, "path": {"type": "string"}, "rev": {"type": "string"},
             }, "required": ["repo", "path"]}, _git_text("blame")),
        Tool("git_diff",
             "Diff between two revs (optionally one path). A large diff returns a "
             "--stat file summary instead of the full patch; pass a 'path' for one "
             "file's full diff, or raise 'max_tokens'.",
             {"type": "object", "properties": {
                 "repo": _REPO, "from_rev": {"type": "string"}, "to_rev": {"type": "string"},
                 "path": {"type": "string"}, "max_tokens": _MAXTOK, "format": _FMT,
             }, "required": ["repo", "from_rev", "to_rev"]}, _git_text("diff")),
        Tool("git_show", "Show a commit (stat + metadata).",
             {"type": "object", "properties": {"repo": _REPO, "rev": {"type": "string"}},
              "required": ["repo", "rev"]}, _git_text("show")),
        Tool("git_file_at_rev", "Read a file's contents at a specific rev.",
             {"type": "object", "properties": {
                 "repo": _REPO, "path": {"type": "string"}, "rev": {"type": "string"},
             }, "required": ["repo", "path", "rev"]}, _git_text("file_at_rev")),
        Tool("get_overview",
             "High-level repo overview: dirs, entrypoints, README, top symbols (and an LLM "
             "summary if enabled). Read this first to understand a repo fast.",
             {"type": "object", "properties": {"repo": _REPO, "format": _FMT},
              "required": ["repo"]}, get_overview),
        Tool("get_docs",
             "Structural docs (public symbols) for a repo, optionally scoped to a path.",
             {"type": "object", "properties": {
                 "repo": _REPO, "path": {"type": "string"},
                 "limit": _LIMIT, "max_tokens": _MAXTOK, "format": _FMT,
             }, "required": ["repo"]}, get_docs),
        Tool("list_repos", "List the repos this indexer tracks.",
             {"type": "object", "properties": {"format": _FMT}}, list_repos),
    ]


async def dispatch(ctx: IndexerContext, name: str, args: dict[str, Any]) -> str:
    """Look up and run a tool, translating known errors into result text."""
    args = args or {}
    req_fmt = _fmt(ctx, args)
    tool = {t.name: t for t in build_tools()}.get(name)
    if tool is None:
        return fmt.render_obj({"error": f"unknown tool {name!r}"}, req_fmt)
    try:
        return await tool.handler(ctx, args)
    except RepoNotIndexedError as e:
        return fmt.render_obj({"error": str(e)}, req_fmt)
    except KeyError as e:
        return fmt.render_obj({"error": f"missing required argument {e}"}, req_fmt)
    except Exception as e:  # noqa: BLE001 - a tool failure must not break the MCP call
        log.warning("tool %s failed: %s", name, e)
        return fmt.render_obj({"error": f"{name} failed: {e}"}, req_fmt)
