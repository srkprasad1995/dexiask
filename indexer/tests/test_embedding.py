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


# --- Ollama (local sidecar, no API key) ---


class _FakeResponse:
    def __init__(self, embeddings):
        self._embeddings = embeddings

    def raise_for_status(self):
        pass

    def json(self):
        return {"embeddings": self._embeddings}


def _stub_ollama_post(monkeypatch, handler):
    from indexer.embedding import ollama as ollama_mod
    monkeypatch.setattr(ollama_mod.httpx, "post", handler)


def test_ollama_requires_base_url():
    from indexer.embedding.ollama import OllamaProvider
    with pytest.raises(ValueError):
        OllamaProvider(base_url="")


def test_ollama_constructor_is_offline(monkeypatch):
    # Constructing the provider must never touch the network — the sidecar may
    # still be starting when the indexer boots.
    def explode(*a, **k):
        raise AssertionError("network touched in constructor")

    _stub_ollama_post(monkeypatch, explode)
    from indexer.embedding.ollama import OllamaProvider
    OllamaProvider(base_url="http://ollama:11434")


def test_ollama_embeds_documents_and_query(monkeypatch):
    captured = {}

    def post(url, json, timeout):
        captured["url"] = url
        captured["json"] = json
        return _FakeResponse([[0.0] * 8 for _ in json["input"]])

    _stub_ollama_post(monkeypatch, post)
    from indexer.embedding.ollama import OllamaProvider
    p = OllamaProvider(base_url="http://ollama:11434/", model="qwen3-embedding:0.6b", dim=8)
    assert p.embed_documents([]) == []
    docs = p.embed_documents(["a", "b"])
    assert len(docs) == 2 and len(docs[0]) == 8
    assert captured["url"] == "http://ollama:11434/api/embed"
    assert captured["json"]["model"] == "qwen3-embedding:0.6b"
    assert captured["json"]["truncate"] is True
    assert len(p.embed_query("find auth")) == 8


def test_ollama_retries_then_succeeds(monkeypatch):
    state = {"calls": 0}

    def post(url, json, timeout):
        state["calls"] += 1
        if state["calls"] == 1:
            raise RuntimeError("connection refused")
        return _FakeResponse([[0.0] * 8 for _ in json["input"]])

    _stub_ollama_post(monkeypatch, post)
    from indexer.embedding.ollama import OllamaProvider
    p = OllamaProvider(base_url="http://ollama:11434", dim=8, max_retries=3, sleep=lambda s: None)
    assert len(p.embed_query("x")) == 8
    assert state["calls"] == 2


def test_ollama_raises_after_exhausting_retries(monkeypatch):
    def post(url, json, timeout):
        raise RuntimeError("still down")

    _stub_ollama_post(monkeypatch, post)
    from indexer.embedding.ollama import OllamaProvider
    p = OllamaProvider(base_url="http://ollama:11434", dim=8, max_retries=2, sleep=lambda s: None)
    with pytest.raises(RuntimeError):
        p.embed_query("x")


def test_ollama_dim_mismatch_is_not_retried(monkeypatch):
    state = {"calls": 0}

    def post(url, json, timeout):
        state["calls"] += 1
        return _FakeResponse([[0.0] * 768 for _ in json["input"]])

    _stub_ollama_post(monkeypatch, post)
    from indexer.embedding.ollama import OllamaProvider
    p = OllamaProvider(base_url="http://ollama:11434", dim=1024, sleep=lambda s: None)
    with pytest.raises(ValueError, match="768"):
        p.embed_query("x")
    assert state["calls"] == 1  # config error, not transient — no retries


def test_ollama_batches_documents(monkeypatch):
    calls = []

    def post(url, json, timeout):
        calls.append(json["input"])
        return _FakeResponse([[0.0] * 4 for _ in json["input"]])

    _stub_ollama_post(monkeypatch, post)
    from indexer.embedding.ollama import OllamaProvider
    p = OllamaProvider(base_url="http://ollama:11434", dim=4, batch_size=2)
    out = p.embed_documents([f"c{i}" for i in range(5)])
    assert len(out) == 5
    assert [len(c) for c in calls] == [2, 2, 1]


def test_factory_builds_ollama():
    prov = build_provider(Settings(
        embedding_provider="ollama",
        ollama_base_url="http://ollama:11434",
        ollama_embedding_model="qwen3-embedding:0.6b",
        embedding_dim=8,
    ))
    assert prov.dim == 8
    assert prov.model == "qwen3-embedding:0.6b"


# --- "auto" provider resolution: hosted keys beat the local sidecar ---


def test_factory_auto_prefers_voyage_key(monkeypatch):
    _stub_voyage(monkeypatch, {})
    prov = build_provider(Settings(
        embedding_provider="auto", voyage_api_key="k",
        ollama_base_url="http://ollama:11434", embedding_dim=8,
    ))
    from indexer.embedding.voyage import VoyageProvider
    assert isinstance(prov, VoyageProvider)


def test_factory_auto_falls_back_to_openai_key(monkeypatch):
    mod = types.ModuleType("openai")

    class OpenAI:
        def __init__(self, api_key):
            pass

    mod.OpenAI = OpenAI
    monkeypatch.setitem(sys.modules, "openai", mod)
    prov = build_provider(Settings(
        embedding_provider="auto", openai_api_key="k",
        ollama_base_url="http://ollama:11434", embedding_dim=8,
    ))
    from indexer.embedding.openai import OpenAIProvider
    assert isinstance(prov, OpenAIProvider)


def test_factory_auto_falls_back_to_ollama():
    prov = build_provider(Settings(
        embedding_provider="auto", ollama_base_url="http://ollama:11434", embedding_dim=8,
    ))
    from indexer.embedding.ollama import OllamaProvider
    assert isinstance(prov, OllamaProvider)


def test_factory_auto_with_nothing_configured_raises():
    with pytest.raises(ValueError, match="no embedding provider configured"):
        build_provider(Settings(
            embedding_provider="auto",
            voyage_api_key="", openai_api_key="", ollama_base_url="",
        ))
