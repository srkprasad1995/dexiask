"""Embedding providers + factory, with the SDK clients stubbed (no network)."""
import sys
import types

import pytest

from indexer.embedding import build_provider
from indexer.settings import Settings


def _stub_voyage(monkeypatch, captured):
    mod = types.ModuleType("voyageai")

    class Client:
        def __init__(self, api_key):
            captured["key"] = api_key

        def embed(self, texts, model, input_type, output_dimension):
            captured["input_type"] = input_type
            return types.SimpleNamespace(embeddings=[[0.0] * output_dimension for _ in texts])

    mod.Client = Client
    monkeypatch.setitem(sys.modules, "voyageai", mod)


def test_voyage_requires_key(monkeypatch):
    _stub_voyage(monkeypatch, {})
    from indexer.embedding.voyage import VoyageProvider
    with pytest.raises(ValueError):
        VoyageProvider(api_key="")


def test_voyage_embeds_documents_and_query(monkeypatch):
    captured = {}
    _stub_voyage(monkeypatch, captured)
    from indexer.embedding.voyage import VoyageProvider
    p = VoyageProvider(api_key="k", model="voyage-code-3", dim=8)
    assert p.embed_documents([]) == []
    docs = p.embed_documents(["a", "b"])
    assert len(docs) == 2 and len(docs[0]) == 8
    assert captured["input_type"] == "document"
    q = p.embed_query("find auth")
    assert len(q) == 8
    assert captured["input_type"] == "query"


def test_factory_builds_voyage(monkeypatch):
    _stub_voyage(monkeypatch, {})
    prov = build_provider(Settings(embedding_provider="voyage", voyage_api_key="k", embedding_dim=8))
    assert prov.dim == 8


def test_factory_unknown_provider():
    with pytest.raises(ValueError):
        build_provider(Settings(embedding_provider="bogus"))


def test_openai_requires_key(monkeypatch):
    mod = types.ModuleType("openai")

    class OpenAI:
        def __init__(self, api_key):
            self.api_key = api_key

    mod.OpenAI = OpenAI
    monkeypatch.setitem(sys.modules, "openai", mod)
    from indexer.embedding.openai import OpenAIProvider
    with pytest.raises(ValueError):
        OpenAIProvider(api_key="")


def test_openai_embeds(monkeypatch):
    mod = types.ModuleType("openai")

    class OpenAI:
        def __init__(self, api_key):
            self.embeddings = self

        def create(self, input, model, dimensions):
            return types.SimpleNamespace(
                data=[types.SimpleNamespace(embedding=[1.0] * dimensions) for _ in input]
            )

    mod.OpenAI = OpenAI
    monkeypatch.setitem(sys.modules, "openai", mod)
    from indexer.embedding.openai import OpenAIProvider
    p = OpenAIProvider(api_key="k", dim=4)
    assert p.embed_documents([]) == []
    assert len(p.embed_query("x")) == 4
    prov = build_provider(Settings(embedding_provider="openai", openai_api_key="k", embedding_dim=4))
    assert prov.dim == 4


def test_hash_provider_offline():
    from indexer.embedding.hashprovider import HashEmbeddingProvider
    p = HashEmbeddingProvider(dim=32)
    assert p.dim == 32
    docs = p.embed_documents(["authenticate user", "render template"])
    assert len(docs) == 2 and len(docs[0]) == 32
    # deterministic
    assert p.embed_query("authenticate user") == docs[0]


def test_factory_builds_hash():
    prov = build_provider(Settings(embedding_provider="hash", embedding_dim=32))
    assert prov.dim == 32


def test_voyage_retries_then_succeeds(monkeypatch):
    import types as _t
    mod = _t.ModuleType("voyageai")
    state = {"calls": 0}

    class Client:
        def __init__(self, api_key):
            pass

        def embed(self, texts, model, input_type, output_dimension):
            state["calls"] += 1
            if state["calls"] < 3:
                raise RuntimeError("rate limit: 3 RPM")
            return _t.SimpleNamespace(embeddings=[[0.0] * output_dimension for _ in texts])

    mod.Client = Client
    monkeypatch.setitem(sys.modules, "voyageai", mod)
    from indexer.embedding.voyage import VoyageProvider
    p = VoyageProvider(api_key="k", dim=8, max_retries=4, sleep=lambda s: None)
    out = p.embed_documents(["x"])  # fails twice, succeeds on 3rd
    assert len(out) == 1 and state["calls"] == 3


def _stub_voyage_capturing(monkeypatch, calls):
    mod = types.ModuleType("voyageai")

    class Client:
        def __init__(self, api_key):
            pass

        def embed(self, texts, model, input_type, output_dimension):
            calls.append(list(texts))
            return types.SimpleNamespace(embeddings=[[0.0] * output_dimension for _ in texts])

    mod.Client = Client
    monkeypatch.setitem(sys.modules, "voyageai", mod)


def test_prepare_batches_splits_by_count_and_preserves_order():
    from indexer.embedding._batching import prepare_batches

    batches = list(prepare_batches(
        ["a", "b", "c", "d", "e"], max_items=2, max_tokens=10_000, max_chars=1000
    ))
    assert batches == [["a", "b"], ["c", "d"], ["e"]]


def test_prepare_batches_splits_by_tokens():
    from indexer.embedding._batching import prepare_batches

    # each text ~ 25 tokens (100 chars / 4); cap 60 tokens => 2 per batch.
    texts = ["x" * 100 for _ in range(3)]
    batches = list(prepare_batches(texts, max_items=999, max_tokens=60, max_chars=1000))
    assert [len(b) for b in batches] == [2, 1]


def test_prepare_batches_truncates_oversized_input():
    from indexer.embedding._batching import prepare_batches

    [batch] = list(prepare_batches(["y" * 5000], max_items=10, max_tokens=10_000, max_chars=100))
    assert batch == ["y" * 100]


def test_voyage_batches_large_document_set(monkeypatch):
    calls: list[list[str]] = []
    _stub_voyage_capturing(monkeypatch, calls)
    from indexer.embedding.voyage import VoyageProvider

    p = VoyageProvider(api_key="k", dim=8, batch_size=2)
    out = p.embed_documents([f"chunk {i}" for i in range(5)])
    assert len(out) == 5  # order/length preserved across batches
    assert [len(c) for c in calls] == [2, 2, 1]  # 3 requests, not 1


def test_voyage_truncates_single_oversized_chunk(monkeypatch):
    calls: list[list[str]] = []
    _stub_voyage_capturing(monkeypatch, calls)
    from indexer.embedding.voyage import VoyageProvider

    p = VoyageProvider(api_key="k", dim=8, max_input_chars=50)
    p.embed_documents(["z" * 10_000])
    assert calls == [["z" * 50]]  # truncated to the per-input char cap


def test_voyage_query_is_truncated(monkeypatch):
    calls: list[list[str]] = []
    _stub_voyage_capturing(monkeypatch, calls)
    from indexer.embedding.voyage import VoyageProvider

    p = VoyageProvider(api_key="k", dim=8, max_input_chars=20)
    p.embed_query("q" * 100)
    assert calls == [["q" * 20]]


def test_factory_passes_batch_size_to_voyage(monkeypatch):
    _stub_voyage(monkeypatch, {})
    prov = build_provider(
        Settings(embedding_provider="voyage", voyage_api_key="k", embedding_dim=8,
                 embedding_batch_size=7)
    )
    assert prov._batch_size == 7


def test_openai_batches_documents(monkeypatch):
    calls: list[list[str]] = []
    mod = types.ModuleType("openai")

    class OpenAI:
        def __init__(self, api_key):
            self.embeddings = self

        def create(self, input, model, dimensions):
            calls.append(list(input))
            return types.SimpleNamespace(
                data=[types.SimpleNamespace(embedding=[1.0] * dimensions) for _ in input]
            )

    mod.OpenAI = OpenAI
    monkeypatch.setitem(sys.modules, "openai", mod)
    from indexer.embedding.openai import OpenAIProvider

    p = OpenAIProvider(api_key="k", dim=4, batch_size=2)
    out = p.embed_documents([f"c{i}" for i in range(5)])
    assert len(out) == 5
    assert [len(c) for c in calls] == [2, 2, 1]


def test_voyage_raises_after_exhausting_retries(monkeypatch):
    import types as _t
    mod = _t.ModuleType("voyageai")

    class Client:
        def __init__(self, api_key):
            pass

        def embed(self, *a, **k):
            raise RuntimeError("still rate limited")

    mod.Client = Client
    monkeypatch.setitem(sys.modules, "voyageai", mod)
    from indexer.embedding.voyage import VoyageProvider
    p = VoyageProvider(api_key="k", dim=8, max_retries=2, sleep=lambda s: None)
    with pytest.raises(RuntimeError):
        p.embed_query("x")
