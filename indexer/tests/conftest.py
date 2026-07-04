"""Shared fixtures: a tiny multi-branch git repo built on disk per test."""
from __future__ import annotations

import subprocess
from pathlib import Path

import pytest


def _git(repo: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(repo), *args], check=True, capture_output=True)


def _commit(repo: Path, msg: str) -> None:
    _git(repo, "add", "-A")
    _git(repo, "commit", "-m", msg)


@pytest.fixture
def sample_repo(tmp_path: Path) -> Path:
    """
    Build a working repo with this history and return its path:

      main:  C1 (a.py, b.py)  ->  C2 (edit a.py, add c.py)
      feat:  branched from C1, edits b.py, deletes a.py
    """
    repo = tmp_path / "sample"
    repo.mkdir()
    _git(repo, "init", "-q", "-b", "main")
    _git(repo, "config", "user.email", "t@example.com")
    _git(repo, "config", "user.name", "Test")

    (repo / "a.py").write_text("def alpha():\n    return 1\n")
    (repo / "b.py").write_text("def beta():\n    return 2\n")
    _commit(repo, "C1")

    # Branch feat from C1.
    _git(repo, "branch", "feat")

    # C2 on main: edit a.py, add c.py.
    (repo / "a.py").write_text("def alpha():\n    return 10\n")
    (repo / "c.py").write_text("def gamma():\n    return 3\n")
    _commit(repo, "C2")

    # feat: edit b.py, delete a.py.
    _git(repo, "checkout", "-q", "feat")
    (repo / "b.py").write_text("def beta():\n    return 20\n")
    (repo / "a.py").unlink()
    _commit(repo, "feat work")

    _git(repo, "checkout", "-q", "main")
    # copy: an exact duplicate of main's tip (every blob identical → full dedup).
    _git(repo, "branch", "copy")
    return repo
