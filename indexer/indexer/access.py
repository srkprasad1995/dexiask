"""
Per-user repo access gating — enforced **in the indexer**, not trusted from a
client-supplied list.

The indexer is the trust boundary: it validates the caller's GitHub token against
each repo itself, so a user who reaches the indexer directly (or forges headers)
can only ever see repos their own token grants. The backend forwards, per request:

  - ``X-Internal-Token``  shared secret proving the request came from the backend
  - ``X-Repo-Access: all`` the caller is unrestricted (admin / auth disabled) —
                          honoured ONLY when the internal token matches
  - ``X-User-Token``      the caller's GitHub token, validated live per repo

Gating is active only when the indexer has ``DEXIASK_INTERNAL_TOKEN`` configured;
otherwise (single-user / no-auth) everything is unrestricted.
"""
from __future__ import annotations

from contextvars import ContextVar
from dataclasses import dataclass


@dataclass(frozen=True)
class RequestAuth:
    internal_ok: bool
    unrestricted: bool
    user_token: str | None


_auth: ContextVar[RequestAuth | None] = ContextVar("req_auth", default=None)


def current_auth() -> RequestAuth | None:
    return _auth.get()


class AccessMiddleware:
    """Pure-ASGI middleware: parse the access headers into a RequestAuth context
    var for the lifetime of each HTTP request. Pure ASGI (not BaseHTTPMiddleware)
    so the context var reliably propagates to the endpoint + MCP tool handlers."""

    def __init__(self, app, internal_token: str = ""):
        self.app = app
        self.internal_token = internal_token

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        headers = {k: v for k, v in (scope.get("headers") or [])}
        internal = headers.get(b"x-internal-token", b"").decode("latin-1")
        internal_ok = bool(self.internal_token) and internal == self.internal_token
        unrestricted = headers.get(b"x-repo-access", b"").decode("latin-1") == "all"
        raw_token = headers.get(b"x-user-token", b"").decode("latin-1").strip()
        ra = RequestAuth(
            internal_ok=internal_ok,
            unrestricted=unrestricted,
            user_token=raw_token or None,
        )
        token = _auth.set(ra)
        try:
            await self.app(scope, receive, send)
        finally:
            _auth.reset(token)


def allowed_repo_ids(ctx) -> frozenset[str] | None:
    """The repo ids the current request may read, or None when unrestricted.

    Unrestricted when the indexer has no internal token configured (single-user /
    no-auth) or the backend asserts it via a valid internal token. Otherwise the
    caller's GitHub token is validated against each git repo (cached); local /
    non-GitHub repos and unverifiable repos are excluded.
    """
    settings = ctx.settings
    if not settings.internal_token:
        return None  # gating disabled — no shared secret configured
    ra = current_auth()
    if ra is None:
        return frozenset()  # gating on, no trusted context → deny
    if ra.internal_ok and ra.unrestricted:
        return None
    if ra.user_token:
        from .github import repos_for_token

        return repos_for_token(ctx, ra.user_token)
    return frozenset()


def repo_allowed(ctx, repo_id: str) -> bool:
    a = allowed_repo_ids(ctx)
    return a is None or repo_id in a


def filter_allowed(ctx, repo_ids: list[str]) -> list[str]:
    a = allowed_repo_ids(ctx)
    if a is None:
        return repo_ids
    return [r for r in repo_ids if r in a]
