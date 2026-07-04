"""Tests for generated domain-knowledge docs: parsing, embedding, search, cache."""
from __future__ import annotations

from pathlib import Path

from indexer.config import IndexerConfig, RepoConfig
from indexer.context import IndexerContext
from indexer.docs import DomainDoc, load_domain_docs, save_domain_docs
from indexer.docs.domain import _parse_docs, generate_domain_docs
from indexer.mcp.tools import _search_hits
from indexer.pipeline import InMemoryStateStore
from indexer.pipeline.indexer import Pipeline
from indexer.settings import Settings
from indexer.store import CONTENT_DOC, QdrantStore, doc_virtual_path, point_id
from indexer.store.schema import build_doc_payload

from .fakes import FakeProvider

# --- schema ---------------------------------------------------------------

def test_doc_payload_and_deterministic_id():
    path = doc_virtual_path("architecture")
    assert path.startswith(".dexiask-docs/")
    payload = build_doc_payload(
        "r", path=path, title="Architecture", category="architecture", ordinal=0, text="body"
    )
    assert payload["content_type"] == CONTENT_DOC
    assert payload["title"] == "Architecture"
    assert payload["lang"] == ""
    # Deterministic id → idempotent upsert.
    assert point_id("r", path, 0) == point_id("r", path, 0)


# --- parsing --------------------------------------------------------------

def test_parse_docs_valid():
    raw = (
        '[{"title":"Request Lifecycle","category":"data-flow","body":"para one\\n\\npara two"},'
        '{"title":"Modules","category":"module","body":"m"}]'
    )
    docs = _parse_docs(raw)
    assert [d.title for d in docs] == ["Request Lifecycle", "Modules"]
    assert docs[0].category == "data-flow"
    assert docs[0].slug == "request-lifecycle"


def test_parse_docs_strips_fence_and_bad_category():
    raw = '```json\n[{"title":"X","category":"bogus","body":"b"}]\n```'
    docs = _parse_docs(raw)
    assert len(docs) == 1
    assert docs[0].category == "concept"  # unknown category normalised


def test_parse_docs_dedupes_slugs():
    raw = '[{"title":"Auth","category":"concept","body":"a"},{"title":"Auth","category":"concept","body":"b"}]'
    docs = _parse_docs(raw)
    assert {d.slug for d in docs} == {"auth", "auth-2"}


def test_parse_docs_invalid_returns_empty():
    assert _parse_docs("not json at all") == []
    assert _parse_docs('{"not":"a list"}') == []


def test_generate_disabled_returns_empty():
    # enable_domain_docs defaults False → no LLM call, empty list.
    assert generate_domain_docs(Settings(anthropic_api_key="k"), {"dirs": []}) == []


# --- embedding + search ---------------------------------------------------

def _pipeline(tmp_path: Path) -> tuple[Pipeline, RepoConfig, IndexerContext, QdrantStore]:
    settings = Settings(workspace_root=str(tmp_path), data_dir=str(tmp_path / "data"))
    repo = RepoConfig(id="r", path=str(tmp_path), primary_branch="main")
    registry = IndexerConfig(repos=[repo])
    store = QdrantStore(location=":memory:")
    embedder = FakeProvider(dim=16)
    pipeline = Pipeline(settings, store, embedder, InMemoryStateStore())
    ctx = IndexerContext(settings, registry, store=store, embedder=embedder)
    return pipeline, repo, ctx, store


def test_embed_docs_and_full_replace_gc(tmp_path):
    pipeline, repo, _ctx, store = _pipeline(tmp_path)

    n = pipeline.embed_docs(repo, [
        DomainDoc("Architecture", "architecture", "architecture", "how it fits together"),
        DomainDoc("Modules", "module", "modules", "the main modules"),
    ])
    assert n >= 2
    after_two = store.count(repo.collection)

    # Regenerating with a smaller set must drop the removed doc's points (GC).
    pipeline.embed_docs(repo, [DomainDoc("Architecture", "architecture", "architecture", "updated")])
    assert store.count(repo.collection) < after_two


def test_docs_survive_code_only_search(tmp_path):
    pipeline, repo, ctx, _store = _pipeline(tmp_path)
    pipeline.embed_docs(repo, [
        DomainDoc("Payments", "concept", "payments", "how payments and billing work"),
    ])

    # code_only=True (the default) must NOT drop generated docs.
    hits = _search_hits(ctx, {"query": "payments billing", "code_only": True})
    assert hits, "expected the domain doc to survive code_only search"
    assert all(h.payload.get("content_type") == CONTENT_DOC for _rid, h in hits)

    # content_type filter narrows to docs.
    only_docs = _search_hits(ctx, {"query": "payments", "content_type": "doc"})
    assert all(h.payload.get("content_type") == CONTENT_DOC for _rid, h in only_docs)


# --- cache round trip -----------------------------------------------------

def test_domain_docs_store_round_trip(tmp_path):
    payload = [{"title": "T", "category": "concept", "slug": "t", "body": "b"}]
    save_domain_docs(str(tmp_path), "r", "main", payload)
    assert load_domain_docs(str(tmp_path), "r", "main") == payload
    # Missing → empty list, not an error.
    assert load_domain_docs(str(tmp_path), "other", "main") == []


# --- qdrant migration -----------------------------------------------------

def test_ensure_collection_content_type_index_idempotent(tmp_path):
    store = QdrantStore(location=":memory:")
    store.ensure_collection("c", 16)
    # Calling again on a pre-existing collection must not raise (idempotent index).
    store.ensure_collection("c", 16)
