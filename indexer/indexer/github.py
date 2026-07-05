"""
GitHub repo-access validation for per-user gating.

Given a caller's GitHub token, resolve which registered repos they may read by
asking the GitHub API (``GET /repos/{owner}/{repo}`` → 200). Results are cached
per (token, repo) with a short TTL so repeated searches don't hammer the API.
"""
from __future__ import annotations

import re
import time
import urllib.error
import urllib.request

_GITHUB_API = "https://api.github.com"
_CACHE_TTL = 300.0  # seconds

# (token, owner/repo) -> (allowed, expires_at)
_cache: dict[tuple[str, str], tuple[bool, float]] = {}

_GH_URL = re.compile(
    r"^(?:https?://github\.com/|git@github\.com:)([^/]+)/([^/]+?)(?:\.git)?/?$"
)


def parse_github_url(url: str) -> tuple[str, str] | None:
    m = _GH_URL.match((url or "").strip())
    if not m:
        return None
    return m.group(1), m.group(2)


def has_access(token: str, owner: str, repo: str) -> bool:
    key = (token, f"{owner}/{repo}")
    now = time.monotonic()
    hit = _cache.get(key)
    if hit is not None and hit[1] > now:
        return hit[0]
    allowed = _check(token, owner, repo)
    _cache[key] = (allowed, now + _CACHE_TTL)
    return allowed


def _check(token: str, owner: str, repo: str) -> bool:
    req = urllib.request.Request(f"{_GITHUB_API}/repos/{owner}/{repo}")
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    try:
        with urllib.request.urlopen(req, timeout=8) as resp:
            return 200 <= resp.status < 300
    except urllib.error.HTTPError:
        # 404 / 403 → the token cannot see the repo.
        return False
    except Exception:
        # Network/other error → fail closed (deny) so gating never leaks on error.
        return False


def repos_for_token(ctx, token: str) -> frozenset[str]:
    """The set of registered repo ids the token can read. Local / non-GitHub
    repos are excluded (they cannot be validated against a token)."""
    allowed: set[str] = set()
    for r in ctx.registry.repos:
        if not r.url:
            continue  # local /workspace path — not token-validatable
        parsed = parse_github_url(r.url)
        if parsed is None:
            continue  # non-GitHub remote — cannot validate
        owner, repo = parsed
        if has_access(token, owner, repo):
            allowed.add(r.id)
    return frozenset(allowed)
