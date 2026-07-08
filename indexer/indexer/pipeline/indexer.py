"""
The indexing pipeline.

``reconcile`` makes the index a pure function of the primary branch's tip: it
embeds paths whose content changed, and deletes paths that disappeared. It is
convergent (repairs drift) and idempotent (deterministic point ids + a
skip-unchanged check), so re-running — including on startup — is safe.
"""
from __future__ import annotations

import re
from dataclasses import dataclass
from functools import lru_cache

from ..chunking import chunk_blob, detect_language
from ..config import RepoConfig
from ..docs import DomainDoc
from ..embedding.base import EmbeddingProvider
from ..git import GitRepo
from ..settings import Settings
from ..store import (
    CONTENT_DOC,
    Chunk,
    QdrantStore,
    build_doc_point,
    build_point,
    doc_virtual_path,
)
from .progress import PHASE_EMBEDDING, ProgressStore
from .state import StateStore


@dataclass
class IndexResult:
    branch: str
    commit: str
    embedded: int  # paths (re)embedded this pass
    removed: int  # paths dropped this pass


class Pipeline:
    def __init__(
        self,
        settings: Settings,
        store: QdrantStore,
        embedder: EmbeddingProvider,
        state: StateStore,
        progress: ProgressStore | None = None,
    ) -> None:
        self.settings = settings
        self.store = store
        self.embedder = embedder
        self.state = state
        self.progress = progress

    # ------------------------------------------------------------ filtering
    def _included(self, repo: RepoConfig, path: str) -> bool:
        parts = path.split("/")
        if any(seg in self.settings.skip_dirs for seg in parts):
            return False
        if repo.subpath:
            sp = repo.subpath.strip("/")
            if not (path == sp or path.startswith(sp + "/")):
                return False
        if any(_match(path, pat) for pat in repo.exclude):
            return False
        return any(_match(path, pat) for pat in repo.include)

    # ------------------------------------------------------------ embedding
    def _embed_path(self, repo: RepoConfig, git: GitRepo, blob_sha: str, path: str) -> int:
        content = git.read_blob(blob_sha)
        if len(content) > self.settings.max_file_bytes:
            return 0
        raw = chunk_blob(content, path)
        if not raw:
            return 0
        lang = detect_language(path) or "text"
        vectors = self.embedder.embed_documents([c.text for c in raw])
        points = [
            build_point(
                repo.id,
                Chunk(blob_sha, i, c.text, lang, c.symbol, c.symbol_kind, c.start_line, c.end_line),
                vec,
                path,
            )
            for i, (c, vec) in enumerate(zip(raw, vectors, strict=True))
        ]
        # Clear any stale chunks for this path (e.g. an edit that shrank the file)
        # before writing the fresh set.
        self.store.delete_by_path(repo.collection, path)
        self.store.upsert(repo.collection, points)
        return len(points)

    # ------------------------------------------------------------ domain docs
    def embed_docs(self, repo: RepoConfig, docs: list[DomainDoc]) -> int:
        """Embed generated domain-knowledge docs into the repo collection as
        ``content_type="doc"`` points. Fully replaces the prior doc set (docs are
        regenerated wholesale), so this also GCs docs that disappeared."""
        self.store.ensure_collection(repo.collection, self.embedder.dim)
        # Full replace: drop the previous doc points before writing the new set.
        self.store.delete_by_content_type(repo.collection, CONTENT_DOC)
        if not docs:
            return 0

        points = []
        for doc in docs:
            path = doc_virtual_path(doc.slug)
            raw = chunk_blob(doc.body.encode("utf-8"), f"{doc.slug}.md")
            texts = [c.text for c in raw] or [doc.body]
            vectors = self.embedder.embed_documents(texts)
            for i, (text, vec) in enumerate(zip(texts, vectors, strict=True)):
                points.append(
                    build_doc_point(
                        repo.id,
                        path=path,
                        title=doc.title,
                        category=doc.category,
                        ordinal=i,
                        text=text,
                        vector=vec,
                    )
                )
        self.store.upsert(repo.collection, points)
        return len(points)

    # ------------------------------------------------------------ entrypoint
    def reconcile(self, repo: RepoConfig, git: GitRepo, *, full: bool = False) -> IndexResult:
        """Make the index match the primary-branch tip exactly."""
        # Unpinned repos follow the mirror's default branch (its HEAD).
        branch = repo.primary_branch or git.head_branch()
        self.store.ensure_collection(repo.collection, self.embedder.dim)
        tip = git.resolve(branch)
        desired: dict[str, str] = {
            e.path: e.sha
            for e in git.ls_tree(branch)
            if e.type == "blob" and self._included(repo, e.path)
        }
        actual = self.state.indexed_paths(repo.id)
        skip = {} if full else actual

        if self.progress is not None:
            self.progress.set_phase(repo.id, PHASE_EMBEDDING)
            self.progress.set_total(repo.id, len(desired))

        embedded = removed = 0
        for path, sha in desired.items():
            if self.progress is not None:
                self.progress.advance(repo.id)
            if skip.get(path) == sha:
                continue
            if self._embed_path(repo, git, sha, path):
                self.state.set_path(repo.id, path, sha)
                embedded += 1
            else:
                # Binary / too large / no chunks → make sure it isn't indexed.
                self.store.delete_by_path(repo.collection, path)
                self.state.remove_path(repo.id, path)
        for path in list(actual):
            if path not in desired:
                self.store.delete_by_path(repo.collection, path)
                self.state.remove_path(repo.id, path)
                removed += 1

        self.state.set_commit(repo.id, tip)
        return IndexResult(branch, tip, embedded, removed)


@lru_cache(maxsize=512)
def _glob_regex(pattern: str) -> re.Pattern:
    """Translate a glob with ``**`` (any, incl. /) and ``*`` (any but /) to regex."""
    out: list[str] = []
    i, n = 0, len(pattern)
    while i < n:
        if pattern[i : i + 2] == "**":
            out.append(".*")
            i += 2
            if i < n and pattern[i] == "/":
                i += 1
        elif pattern[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pattern[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pattern[i]))
            i += 1
    return re.compile("^" + "".join(out) + "$")


def _match(path: str, pattern: str) -> bool:
    rx = _glob_regex(pattern)
    return bool(rx.match(path) or rx.match(path.rsplit("/", 1)[-1]))
