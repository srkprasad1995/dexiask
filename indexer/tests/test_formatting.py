import json

from indexer.mcp import formatting as fmt


def test_encode_toon_uniform_list_envelope():
    rows = [
        {"path": "a.py", "lines": "1-2", "symbol": "alpha", "score": 0.9},
        {"path": "b.py", "lines": "1-2", "symbol": "beta", "score": 0.8},
    ]
    out = fmt.render_results(rows, fmt="toon", limit=10, max_tokens=1000)
    assert "results[2]{path,lines,symbol,score}:" in out
    assert "a.py,1-2,alpha,0.9" in out
    assert "total: 2" in out
    assert "truncated: false" in out


def test_encode_json_roundtrips():
    rows = [{"path": "a.py", "score": 1}]
    out = fmt.render_results(rows, fmt="json", limit=10, max_tokens=1000)
    data = json.loads(out)
    assert data["results"][0]["path"] == "a.py"
    assert data["total"] == 1


def test_quotes_values_with_delimiters():
    rows = [{"snippet": "a, b: c", "n": 1}]
    out = fmt.render_results(rows, fmt="toon", limit=10, max_tokens=1000)
    assert '"a, b: c"' in out


def test_fit_rows_truncates_on_limit():
    rows = [{"x": i} for i in range(50)]
    kept, truncated = fmt.fit_rows(rows, limit=5, max_tokens=10_000)
    assert len(kept) == 5
    assert truncated is True


def test_fit_rows_truncates_on_token_budget():
    rows = [{"text": "x" * 400} for _ in range(20)]  # ~100 tokens each
    kept, truncated = fmt.fit_rows(rows, limit=20, max_tokens=250)
    assert 0 < len(kept) < 20
    assert truncated is True


def test_fit_rows_keeps_at_least_one():
    rows = [{"text": "x" * 10_000}]
    kept, truncated = fmt.fit_rows(rows, limit=10, max_tokens=1)
    assert len(kept) == 1
    assert truncated is False


def test_estimate_tokens():
    assert fmt.estimate_tokens("") == 0
    assert fmt.estimate_tokens("abcd") == 1
    assert fmt.estimate_tokens("abcde") == 2
