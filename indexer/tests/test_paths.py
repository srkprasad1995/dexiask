import pytest

from indexer.paths import docs_cache_path, mirror_path, resolve_repo_path


def test_resolve_relative_under_workspace(tmp_path):
    p = resolve_repo_path(str(tmp_path), "myrepo")
    assert p == (tmp_path / "myrepo").resolve()


def test_resolve_absolute_inside_root_kept(tmp_path):
    inside = tmp_path / "r"
    p = resolve_repo_path(str(tmp_path), str(inside))
    assert p == inside.resolve()


def test_resolve_rejects_traversal(tmp_path):
    with pytest.raises(ValueError):
        resolve_repo_path(str(tmp_path), "../../etc")


def test_mirror_path(tmp_path):
    m = mirror_path(str(tmp_path), "repo1")
    assert m.name == "repo1.git"
    assert m.parent.name == "mirrors"


def test_docs_cache_path(tmp_path):
    d = docs_cache_path(str(tmp_path), "repo1")
    assert d.name == "repo1"
    assert d.parent.name == "docs"
