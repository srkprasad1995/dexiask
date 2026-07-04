"""Git layer: bare-mirror management and read-only plumbing against refs."""

from .repo import Change, GitError, GitRepo, GrepHit, TreeEntry

__all__ = ["Change", "GitError", "GitRepo", "GrepHit", "TreeEntry"]
