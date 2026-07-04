from indexer.store import Chunk, QdrantStore, build_point, point_id

from .fakes import FakeProvider


def _store_with_points():
    emb = FakeProvider(dim=16)
    store = QdrantStore(location=":memory:")
    store.ensure_collection("chunks_r", emb.dim)

    chunks = [
        Chunk(blob_sha="aaa", ordinal=0, text="def authenticate user password", lang="py",
              symbol="authenticate", symbol_kind="function", start_line=1, end_line=5),
        Chunk(blob_sha="bbb", ordinal=0, text="def render template html", lang="py",
              symbol="render", symbol_kind="function", start_line=1, end_line=4),
    ]
    vectors = emb.embed_documents([c.text for c in chunks])
    store.upsert("chunks_r", [
        build_point("r", chunks[0], vectors[0], "auth.py"),
        build_point("r", chunks[1], vectors[1], "view/render.py"),
    ])
    return emb, store


def test_point_id_deterministic():
    assert point_id("r", "auth.py", 0) == point_id("r", "auth.py", 0)
    assert point_id("r", "auth.py", 0) != point_id("r", "auth.py", 1)
    assert point_id("r", "auth.py", 0) != point_id("r", "view.py", 0)


def test_search_returns_relevant_hit():
    emb, store = _store_with_points()
    q = emb.embed_query("authenticate user")
    hits = store.search("chunks_r", q, limit=5)
    assert hits
    assert hits[0].payload["symbol"] == "authenticate"
    assert hits[0].payload["path"] == "auth.py"
    assert hits[0].payload["start_line"] == 1


def test_lang_filter():
    emb, store = _store_with_points()
    q = emb.embed_query("authenticate")
    assert store.search("chunks_r", q, limit=5, lang="py")
    assert store.search("chunks_r", q, limit=5, lang="go") == []


def test_symbol_kind_filter():
    emb, store = _store_with_points()
    q = emb.embed_query("render")
    assert store.search("chunks_r", q, limit=5, symbol_kind="function")
    assert store.search("chunks_r", q, limit=5, symbol_kind="class") == []


def test_path_prefix_filter():
    emb, store = _store_with_points()
    q = emb.embed_query("render template")
    hits = store.search("chunks_r", q, limit=5, path_prefix="view/")
    assert {h.payload["path"] for h in hits} == {"view/render.py"}


def test_delete_by_path_removes_chunks():
    emb, store = _store_with_points()
    assert store.count("chunks_r") == 2
    store.delete_by_path("chunks_r", "auth.py")
    assert store.count("chunks_r") == 1
    q = emb.embed_query("authenticate user")
    assert all(h.payload["path"] != "auth.py" for h in store.search("chunks_r", q, limit=5))


def test_scroll_by_path():
    _emb, store = _store_with_points()
    ids = [pid for pid, _ in store.scroll_by_path("chunks_r", "auth.py")]
    assert len(ids) == 1
    store.delete("chunks_r", ids)
    assert store.count("chunks_r") == 1
