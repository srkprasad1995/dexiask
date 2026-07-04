from indexer.chunking import chunk_blob, detect_language, walk_symbols


class FakeNode:
    def __init__(self, type, start, end, children=(), text=b""):
        self.type = type
        self.start_point = (start, 0)
        self.end_point = (end, 0)
        self.children = list(children)
        self.text = text


def test_detect_language():
    assert detect_language("a/b/main.go") == "go"
    assert detect_language("x.py") == "python"
    assert detect_language("README") is None


def test_binary_yields_nothing():
    assert chunk_blob(b"\x00\x01\x02binary", "x.bin") == []


def test_empty_yields_nothing():
    assert chunk_blob(b"   \n  ", "x.py") == []


def test_small_file_one_chunk():
    chunks = chunk_blob(b"def f():\n    return 1\n", "x.py")
    assert len(chunks) >= 1
    assert chunks[0].start_line == 1


def test_large_file_windows_with_overlap():
    content = ("\n".join(f"line {i}" for i in range(1, 201))).encode()
    chunks = chunk_blob(content, "x.unknownext")  # no language → line-window
    assert len(chunks) > 1
    # windows overlap → second chunk starts before the first ends
    assert chunks[1].start_line <= chunks[0].end_line
    # full coverage to the last line
    assert chunks[-1].end_line == 200


def test_chunks_are_ordered_and_contiguous_cover():
    content = ("\n".join(str(i) for i in range(1, 100))).encode()
    chunks = chunk_blob(content, "x.txt")
    assert chunks[0].start_line == 1
    for a, b in zip(chunks, chunks[1:], strict=False):
        assert b.start_line > a.start_line


def _root(children):
    return FakeNode("module", 0, 100, children)


def test_walk_symbols_extracts_named_symbol():
    ident = FakeNode("identifier", 0, 0, text=b"foo")
    fn = FakeNode("function_definition", 0, 1, children=[ident])  # lines 1-2
    lines = ["def foo():", "    return 1"]
    chunks = walk_symbols(_root([fn]), "python", lines)
    assert len(chunks) == 1
    assert chunks[0].symbol == "foo"
    assert chunks[0].symbol_kind == "function"
    assert (chunks[0].start_line, chunks[0].end_line) == (1, 2)


def test_walk_symbols_skips_non_symbol_nodes():
    other = FakeNode("comment", 0, 0)
    cls = FakeNode("class_definition", 2, 3, children=[FakeNode("identifier", 0, 0, text=b"Bar")])
    chunks = walk_symbols(_root([other, cls]), "python", ["", "", "class Bar:", "    pass"])
    assert [c.symbol_kind for c in chunks] == ["class"]
    assert chunks[0].symbol == "Bar"


def test_walk_symbols_splits_oversized_symbol():
    big = FakeNode("function_definition", 0, 199, children=[FakeNode("identifier", 0, 0, text=b"big")])
    lines = [f"l{i}" for i in range(200)]
    chunks = walk_symbols(_root([big]), "python", lines)
    assert len(chunks) > 1
    assert all(c.symbol == "big" for c in chunks)
