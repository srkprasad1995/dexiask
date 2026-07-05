"""
FastAPI application: the MCP endpoint (/mcp) plus the REST control plane.

The MCP server is mounted at ``/mcp`` over streamable HTTP. The control plane
(``/reindex``, ``/v1/index/{repo}``, ``/v1/repos``, ``/v1/status``, ``/healthz``)
drives and reports on indexing; a background scheduler reconciles on an interval
and performs the initial index on startup.

Stores are built best-effort: if Qdrant or the embedding key is unavailable the
service still starts and serves lexical/git/read tools off the mirror
(semantic_search degrades) — the "independent degradation" property. There is no
database: state and the per-repo lock are in-memory; the repo registry is
persisted to a JSON file on the shared filesystem.
"""
from __future__ import annotations

import asyncio
import logging
import os
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from .config.models import IndexerConfig, RepoConfig
from .config.registry import load_registry_file, merge_repo
from .context import IndexerContext, RepoNotFoundError
from .embedding import build_provider
from .git.repo import GitError
from .lock import InMemoryLock
from .mcp.server import build_session_manager
from .pipeline import InMemoryStateStore
from .registry_store import FsRepoStore, RepoStore
from .scheduler import Scheduler
from .service import IndexService
from .settings import Settings, get_settings
from .store import QdrantStore

log = logging.getLogger("indexer")


def _git_token_path(settings: Settings) -> Path:
    """Location of the persisted UI-set git token on the data volume."""
    return Path(settings.data_dir) / "git_token"


def read_git_token(settings: Settings) -> str:
    """Return the persisted git token, or "" when none is stored."""
    try:
        return _git_token_path(settings).read_text(encoding="utf-8").strip()
    except FileNotFoundError:
        return ""


def write_git_token(settings: Settings, token: str) -> None:
    """Persist the git token with owner-only (0600) permissions."""
    path = _git_token_path(settings)
    path.parent.mkdir(parents=True, exist_ok=True)
    # Create with restrictive perms from the outset so the secret is never
    # briefly world-readable.
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        os.write(fd, token.encode("utf-8"))
    finally:
        os.close(fd)
    os.chmod(path, 0o600)


def delete_git_token(settings: Settings) -> None:
    """Remove the persisted git token file if present."""
    _git_token_path(settings).unlink(missing_ok=True)


def build_components(settings: Settings):
    repo_store: RepoStore = FsRepoStore(Path(settings.data_dir) / "repos.json")

    # A UI-set token persisted on the data volume overrides the env default so
    # it survives restarts. build_repo_mirror() reads settings.git_token, so the
    # scheduler's reconcile and manual reindex both pick this up.
    persisted_token = read_git_token(settings)
    if persisted_token:
        settings.git_token = persisted_token

    # repos.yaml is the declared base; persisted runtime registrations overlay it.
    registry = load_registry_file(settings.repos_config_path)
    for repo in repo_store.list():
        registry = merge_repo(registry, repo)

    store: QdrantStore | None = None
    embedder = None
    try:
        store = QdrantStore(url=settings.qdrant_url)
    except Exception as e:  # pragma: no cover - depends on env
        log.warning("Qdrant unavailable, semantic search disabled: %s", e)
    try:
        embedder = build_provider(settings)
    except Exception as e:  # pragma: no cover - depends on env
        log.warning("Embedder unavailable, semantic search disabled: %s", e)

    ctx = IndexerContext(settings, registry, store=store, embedder=embedder)
    service = IndexService(
        settings, registry, store=store, embedder=embedder,
        state=InMemoryStateStore(), lock=InMemoryLock(),
    )
    return ctx, service, repo_store


def create_app(
    ctx: IndexerContext | None = None,
    service: IndexService | None = None,
    *,
    repo_store: RepoStore | None = None,
    run_scheduler: bool = False,
) -> FastAPI:
    if ctx is None:
        settings = get_settings()
        ctx, service, repo_store = build_components(settings)
    else:
        settings = ctx.settings
        if repo_store is None:
            from .registry_store import InMemoryRepoStore

            repo_store = InMemoryRepoStore()
        if service is None:
            service = IndexService(
                settings, ctx.registry, store=ctx.store, embedder=ctx.embedder,
                state=InMemoryStateStore(), lock=InMemoryLock(),
            )
    manager = build_session_manager(ctx)
    scheduler = Scheduler(service, ctx.registry, settings.reindex_interval_s)

    @asynccontextmanager
    async def lifespan(_app: FastAPI):
        async with manager.run():
            if run_scheduler:
                scheduler.start()
            try:
                yield
            finally:
                if run_scheduler:
                    await scheduler.stop()

    app = FastAPI(title="dexiask-indexer", lifespan=lifespan)
    from .access import AccessMiddleware

    # Per-user repo gating: parse the access headers into a context var for both
    # REST + MCP enforcement. The indexer validates the caller's token itself.
    app.add_middleware(AccessMiddleware, internal_token=settings.internal_token)

    from .observability import init_observability

    init_observability(app)

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/v1/status")
    async def status() -> dict[str, Any]:
        from .access import repo_allowed

        def _collect() -> list[dict[str, Any]]:
            repos = []
            for r in ctx.registry.repos:
                if not repo_allowed(ctx, r.id):
                    continue
                mirror = ctx.mirror_for(r.id)
                entry: dict[str, Any] = {"id": r.id, "indexed": mirror.exists()}
                if mirror.exists():
                    entry["branch"] = r.primary_branch or mirror.tracked_branch()
                    entry["commit"] = service.state.get_commit(r.id)
                repos.append(entry)
            return repos

        return {"status": "ready", "repos": await asyncio.to_thread(_collect)}

    async def _run_index(repo_id: str | None, **kw) -> JSONResponse:
        targets = [repo_id] if repo_id else [r.id for r in ctx.registry.repos]
        results = []
        for rid in targets:
            try:
                results.append(await asyncio.to_thread(service.index_repo, rid, **kw))
            except RepoNotFoundError:
                return JSONResponse({"error": f"unknown repo {rid!r}"}, status_code=404)
            except GitError as e:
                # Clone/fetch failed — almost always a missing/invalid git token for
                # a private repo, or an unreachable remote. Surface it cleanly (with
                # a hint) instead of a 500 so the UI can show something actionable.
                msg = str(e)
                hint = (
                    "could not access the repository — for a private repo, set a git "
                    "token (Indexer → Git access token) or sign in with GitHub."
                    if "could not read Username" in msg or "Authentication failed" in msg
                    else "git operation failed."
                )
                return JSONResponse(
                    {"error": f"failed to index {rid!r}: {hint}", "detail": msg, "repo": rid},
                    status_code=502,
                )
        return JSONResponse({"results": results})

    def _effective_token(request: Request, body: dict[str, Any]) -> str | None:
        # An explicit body token wins; otherwise fall back to the per-user
        # X-Git-Token header the backend injects (the caller's GitHub OAuth token).
        return body.get("token") or request.headers.get("X-Git-Token") or None

    @app.post("/reindex")
    async def reindex(request: Request, body: dict[str, Any] | None = None) -> JSONResponse:
        body = body or {}
        return await _run_index(
            body.get("repo"), full=bool(body.get("full")), token=_effective_token(request, body)
        )

    @app.post("/v1/index/{repo_id}")
    async def index_one(
        repo_id: str, request: Request, body: dict[str, Any] | None = None
    ) -> JSONResponse:
        body = body or {}
        return await _run_index(
            repo_id, full=bool(body.get("full")), token=_effective_token(request, body)
        )

    @app.get("/v1/git-token")
    async def get_git_token() -> dict[str, bool]:
        # Report only whether a token is configured — the value is never returned
        # to the client.
        return {"configured": bool(settings.git_token)}

    @app.put("/v1/git-token")
    async def set_git_token(body: dict[str, Any] | None = None) -> dict[str, bool]:
        body = body or {}
        token = str(body.get("token") or "").strip()
        if not token:
            # Empty → clear the configured token and remove the persisted file.
            settings.git_token = ""
            delete_git_token(settings)
        else:
            settings.git_token = token
            write_git_token(settings, token)
        return {"configured": bool(settings.git_token)}

    @app.get("/v1/repos")
    async def list_repos() -> dict[str, Any]:
        # Every registered repo (git URL or local /workspace path) with its
        # current index status, filtered to the caller's allowed set.
        from .access import repo_allowed

        def _collect() -> list[dict[str, Any]]:
            repos: list[dict[str, Any]] = []
            for r in ctx.registry.repos:
                if not repo_allowed(ctx, r.id):
                    continue
                mirror = ctx.mirror_for(r.id)
                indexed = mirror.exists()
                entry: dict[str, Any] = {
                    "id": r.id,
                    "indexed": indexed,
                    "branch": r.primary_branch or (mirror.tracked_branch() if indexed else ""),
                }
                if r.url:
                    entry["url"] = r.url
                if r.path:
                    entry["path"] = r.path
                if indexed:
                    entry["commit"] = service.state.get_commit(r.id)
                    if ctx.store is not None:
                        try:
                            entry["chunks"] = ctx.store.count(r.collection)
                        except Exception:  # pragma: no cover - depends on store
                            pass
                repos.append(entry)
            return repos

        return {"repos": await asyncio.to_thread(_collect)}

    @app.get("/v1/docs/{repo_id}")
    async def get_domain_docs(repo_id: str) -> JSONResponse:
        # Generated domain-knowledge docs for the web indexer "Docs" tab.
        from .docs import load_domain_docs

        repo = ctx.registry.get(repo_id)
        if repo is None:
            return JSONResponse({"error": f"unknown repo {repo_id!r}"}, status_code=404)
        docs = await asyncio.to_thread(
            load_domain_docs, settings.data_dir, repo.id, ctx.primary_branch(repo.id)
        )
        return JSONResponse({"docs": docs})

    @app.post("/v1/search")
    async def search(body: dict[str, Any] | None = None) -> JSONResponse:
        # Clean JSON semantic search for the web indexer page (the MCP tool
        # returns TOON/pointer-first text; this returns rendered rows).
        body = body or {}
        if not body.get("query"):
            return JSONResponse(
                {"error": "query is required", "results": []}, status_code=400
            )
        from .mcp.tools import search_json

        return JSONResponse(await asyncio.to_thread(search_json, ctx, body))

    @app.post("/v1/repos")
    async def register_repo(repo: dict[str, Any]) -> dict[str, Any]:
        cfg = RepoConfig.model_validate(repo)
        repo_store.upsert(cfg)  # persisted; survives restarts
        ctx.registry = merge_repo(ctx.registry, cfg)
        service.registry = ctx.registry
        return {"registered": cfg.id}

    @app.delete("/v1/repos/{repo_id}")
    async def deregister_repo(repo_id: str) -> dict[str, Any]:
        removed = repo_store.delete(repo_id)
        ctx.registry = IndexerConfig(
            repos=[r for r in ctx.registry.repos if r.id != repo_id],
            default_embedding=ctx.registry.default_embedding,
        )
        service.registry = ctx.registry
        return {"deregistered": repo_id, "removed": removed}

    @app.get("/v1/tools")
    async def list_tools() -> dict[str, Any]:
        from .mcp.tools import build_tools

        return {
            "tools": [
                {"name": t.name, "description": t.description, "input_schema": t.input_schema}
                for t in build_tools()
            ]
        }

    @app.post("/v1/tools/{name}")
    async def run_tool(name: str, body: dict[str, Any] | None = None) -> dict[str, Any]:
        # REST surface over the exact same tool dispatch the MCP server uses.
        from .mcp.tools import dispatch

        result = await dispatch(ctx, name, body or {})
        return {"tool": name, "result": result}

    app.mount("/mcp", manager.handle_request)
    return app
