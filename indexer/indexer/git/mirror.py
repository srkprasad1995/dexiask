"""
Bare-mirror management.

The indexer never reads the mutable working tree. Instead it keeps a bare,
single-branch mirror per repo (``git clone --bare --single-branch``) tracking the
repo's primary branch, and ``git fetch``es it. The source is either:

  - a **local** git directory under ``workspace_root`` (use its ``.git``), or
  - a **remote URL** (https/ssh/git/file) cloned and fetched directly.

For private HTTPS remotes, a token is supplied transiently as an
``Authorization`` header injected via ``GIT_CONFIG_*`` environment variables. It
is therefore never written into the mirror's stored ``remote.origin.url`` and
never appears in the process argument list.
"""
from __future__ import annotations

import base64
import os
import re
import subprocess
from pathlib import Path

from ..config import RepoConfig
from ..paths import mirror_path, resolve_repo_path
from ..settings import Settings
from .repo import GitError, GitRepo

# Schemes we treat as a remote URL to clone from verbatim (vs a local path).
_REMOTE_RE = re.compile(r"^(?:https?|ssh|git|file)://|^[^/]+@[^/]+:")
# Schemes that carry an HTTP Authorization header for token auth.
_HTTP_RE = re.compile(r"^https?://")


def is_remote_url(source: str) -> bool:
    return bool(_REMOTE_RE.match(source))


def build_auth_header(token: str, username: str = "x-access-token") -> str | None:
    """Build a Basic ``Authorization`` header value for a git token, or None."""
    if not token:
        return None
    encoded = base64.b64encode(f"{username}:{token}".encode()).decode("ascii")
    return f"Authorization: Basic {encoded}"


def _git(*args: str, auth_header: str | None = None) -> None:
    env = os.environ.copy()
    if auth_header:
        # Inject http.extraHeader via env (git >= 2.31) so the token never lands
        # in argv (visible to `ps`) nor in the cloned repo's persisted config.
        env["GIT_CONFIG_COUNT"] = "1"
        env["GIT_CONFIG_KEY_0"] = "http.extraHeader"
        env["GIT_CONFIG_VALUE_0"] = auth_header
    proc = subprocess.run(["git", *args], capture_output=True, env=env)
    if proc.returncode != 0:
        raise GitError(
            f"git {' '.join(args)} failed: {proc.stderr.decode('utf-8', 'replace').strip()}"
        )


class Mirror:
    """A bare, single-branch mirror of one repo, fetched from a local git dir or
    a remote URL. Only one branch is tracked (default-branch only), which keeps
    clone time, disk, and every fetch minimal.

    ``branch`` may be ``None`` (or empty) to follow the source's own default
    branch — the mirror is cloned without ``--branch`` so git picks the remote
    HEAD, and the tracked branch is then read back off the mirror's HEAD.
    """

    def __init__(
        self,
        mirror_dir: Path,
        source: str | Path,
        *,
        branch: str | None = None,
        auth_header: str | None = None,
    ) -> None:
        self.mirror_dir = Path(mirror_dir)
        self.branch = branch or None
        self._auth = auth_header
        src = str(source)
        if is_remote_url(src):
            self.source = src  # cloned/fetched verbatim
        else:
            # Local: use the working dir's ``.git`` if present, else the dir itself.
            p = Path(src)
            self.source = str(p / ".git") if (p / ".git").is_dir() else str(p)

    def exists(self) -> bool:
        return (self.mirror_dir / "HEAD").exists()

    def ensure(self) -> None:
        """Create the mirror if missing; otherwise refresh it."""
        if self.exists():
            self.fetch()
            return
        self.mirror_dir.parent.mkdir(parents=True, exist_ok=True)
        # With no pinned branch, omit ``--branch`` so git clones the source's
        # own default branch (its HEAD).
        branch_args = ["--branch", self.branch] if self.branch else []
        _git(
            "clone", "--bare", "--single-branch", *branch_args,
            self.source, str(self.mirror_dir), auth_header=self._auth,
        )

    def tracked_branch(self) -> str:
        """The branch this mirror tracks — the pinned name, or the default read
        off the existing mirror's HEAD."""
        return self.branch or self.repo().head_branch()

    def fetch(self) -> None:
        """Refresh the tracked branch's ref from the source."""
        branch = self.tracked_branch()
        _git(
            "--git-dir", str(self.mirror_dir), "fetch", self.source,
            f"+refs/heads/{branch}:refs/heads/{branch}", auth_header=self._auth,
        )

    def repo(self) -> GitRepo:
        return GitRepo(git_dir=self.mirror_dir)


def build_repo_mirror(
    settings: Settings, repo: RepoConfig, *, token: str | None = None
) -> Mirror:
    """Construct the :class:`Mirror` for a repo, handling remote vs local source.

    For remote HTTPS repos, an auth header is built from the per-request
    ``token`` (if given) or the ``DEXIASK_GIT_TOKEN`` default.
    """
    mdir = mirror_path(settings.data_dir, repo.id)
    if repo.url:
        effective = token or settings.git_token
        header = (
            build_auth_header(effective, settings.git_token_username)
            if effective and _HTTP_RE.match(repo.url)
            else None
        )
        return Mirror(mdir, repo.url, branch=repo.primary_branch or None, auth_header=header)
    src = resolve_repo_path(settings.workspace_root, repo.path)
    return Mirror(mdir, src, branch=repo.primary_branch or None)
