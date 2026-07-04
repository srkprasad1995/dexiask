"""
Read-only git plumbing against a single git directory (a bare mirror).

Everything the indexer needs — tree listing, blob reads, grep, and introspection
(log/blame/diff/show/branches) — is derived from git objects against a ref, so we
never check a branch out. All operations are branch-correct because the ref is
passed explicitly.
"""
from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path


class GitError(RuntimeError):
    """A git invocation failed."""


@dataclass(frozen=True)
class TreeEntry:
    mode: str
    type: str  # "blob" | "tree" | "commit"
    sha: str
    path: str


@dataclass(frozen=True)
class Change:
    status: str  # A | M | D | R | C | T
    path: str  # new path (or the path for A/M/D)
    old_path: str | None  # source path for renames/copies
    old_sha: str
    new_sha: str


@dataclass(frozen=True)
class GrepHit:
    path: str
    lineno: int
    line: str


@dataclass(frozen=True)
class GitRepo:
    git_dir: Path

    # ------------------------------------------------------------------ core
    def _run(self, *args: str, binary: bool = False) -> bytes | str:
        cmd = ["git", "--git-dir", str(self.git_dir), *args]
        proc = subprocess.run(cmd, capture_output=True)
        if proc.returncode != 0:
            raise GitError(
                f"git {' '.join(args)} failed: {proc.stderr.decode('utf-8', 'replace').strip()}"
            )
        return proc.stdout if binary else proc.stdout.decode("utf-8", "replace")

    def _run_ok(self, *args: str) -> tuple[bool, str]:
        """Run a command that may legitimately exit non-zero (e.g. grep no-match)."""
        cmd = ["git", "--git-dir", str(self.git_dir), *args]
        proc = subprocess.run(cmd, capture_output=True)
        return proc.returncode == 0, proc.stdout.decode("utf-8", "replace")

    # ------------------------------------------------------------------ refs
    def resolve(self, ref: str) -> str:
        return self._run("rev-parse", "--verify", f"{ref}^{{commit}}").strip()  # type: ignore[union-attr]

    def branches(self) -> list[str]:
        out = self._run("for-each-ref", "--format=%(refname:short)", "refs/heads")
        return [b for b in out.splitlines() if b]  # type: ignore[union-attr]

    # ------------------------------------------------------------------ trees
    def ls_tree(self, ref: str, subpath: str | None = None) -> list[TreeEntry]:
        args = ["ls-tree", "-r", "-z", ref]
        if subpath:
            args += ["--", subpath]
        out = self._run(*args)
        entries: list[TreeEntry] = []
        for rec in out.split("\0"):  # type: ignore[union-attr]
            if not rec:
                continue
            meta, _, path = rec.partition("\t")
            mode, typ, sha = meta.split()
            entries.append(TreeEntry(mode=mode, type=typ, sha=sha, path=path))
        return entries

    def read_blob(self, blob_sha: str) -> bytes:
        return self._run("cat-file", "-p", blob_sha, binary=True)  # type: ignore[return-value]

    def read_path(self, ref: str, path: str) -> str:
        return self._run("show", f"{ref}:{path}")  # type: ignore[return-value]

    # ------------------------------------------------------------------ search
    def grep(
        self, pattern: str, ref: str, pathspec: str | None = None, max_hits: int = 200
    ) -> list[GrepHit]:
        args = ["grep", "-n", "-I", "-E", pattern, ref]
        if pathspec:
            args += ["--", pathspec]
        ok, out = self._run_ok(*args)
        if not ok:
            return []  # no matches (git grep exits 1) or error → empty
        hits: list[GrepHit] = []
        for line in out.splitlines():
            # Format: "<ref>:<path>:<lineno>:<content>"
            rest = line[len(ref) + 1:] if line.startswith(ref + ":") else line
            path, _, after = rest.partition(":")
            lineno_s, _, content = after.partition(":")
            if not lineno_s.isdigit():
                continue
            hits.append(GrepHit(path=path, lineno=int(lineno_s), line=content))
            if len(hits) >= max_hits:
                break
        return hits

    # ------------------------------------------------------------ introspect
    def log(self, ref: str, path: str | None = None, limit: int = 20) -> list[dict]:
        fmt = "%H%x1f%an%x1f%aI%x1f%s"
        args = ["log", f"--max-count={limit}", f"--format={fmt}", ref]
        if path:
            args += ["--", path]
        out = self._run(*args)
        rows: list[dict] = []
        for line in out.splitlines():  # type: ignore[union-attr]
            if not line:
                continue
            sha, author, date, subject = line.split("\x1f")
            rows.append({"commit": sha, "author": author, "date": date, "subject": subject})
        return rows

    def blame(self, ref: str, path: str) -> str:
        return self._run("blame", "--date=short", ref, "--", path)  # type: ignore[return-value]

    def diff(self, from_rev: str, to_rev: str, path: str | None = None) -> str:
        args = ["diff", from_rev, to_rev]
        if path:
            args += ["--", path]
        return self._run(*args)  # type: ignore[return-value]

    def diff_stat(self, from_rev: str, to_rev: str, path: str | None = None) -> str:
        """The ``--stat`` summary (changed files + line counts) — a compact
        stand-in when the full diff is too large to return to the model."""
        args = ["diff", "--stat", from_rev, to_rev]
        if path:
            args += ["--", path]
        return self._run(*args)  # type: ignore[return-value]

    def show(self, rev: str) -> str:
        return self._run("show", "--stat", "--format=fuller", rev)  # type: ignore[return-value]
