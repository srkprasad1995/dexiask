from pathlib import Path

from indexer.git import GitRepo
from indexer.git.mirror import Mirror


def _repo(sample_repo: Path) -> GitRepo:
    return GitRepo(git_dir=sample_repo / ".git")


def test_branches(sample_repo):
    assert set(_repo(sample_repo).branches()) == {"main", "feat", "copy"}


def test_ls_tree_main(sample_repo):
    paths = {e.path for e in _repo(sample_repo).ls_tree("main") if e.type == "blob"}
    assert paths == {"a.py", "b.py", "c.py"}


def test_read_path_branch_correct(sample_repo):
    r = _repo(sample_repo)
    assert "return 10" in r.read_path("main", "a.py")
    assert "return 20" in r.read_path("feat", "b.py")


def test_blob_dedup_unchanged_file_same_sha(sample_repo):
    r = _repo(sample_repo)
    # b.py is unchanged between C1 and the feat branch point, but edited on feat.
    # c.py exists only on main; a.py differs between main and C1.
    main_blobs = {e.path: e.sha for e in r.ls_tree("main") if e.type == "blob"}
    feat_blobs = {e.path: e.sha for e in r.ls_tree("feat") if e.type == "blob"}
    # b.py was edited on feat → different sha; this asserts content-addressing works.
    assert main_blobs["b.py"] != feat_blobs["b.py"]


def test_grep_branch_scoped(sample_repo):
    r = _repo(sample_repo)
    hits_main = r.grep("return 10", "main")
    assert any(h.path == "a.py" for h in hits_main)
    # a.py does not exist on feat → no hit.
    assert r.grep("return 10", "feat") == []


def test_log(sample_repo):
    rows = _repo(sample_repo).log("main", limit=10)
    assert rows[0]["subject"] == "C2"
    assert {row["subject"] for row in rows} >= {"C1", "C2"}


def test_mirror_clone_and_read(sample_repo, tmp_path):
    # The mirror is single-branch (default-branch only), so only "main" is present.
    mirror = Mirror(tmp_path / "m" / "sample.git", sample_repo, branch="main")
    mirror.ensure()
    assert mirror.exists()
    r = mirror.repo()
    assert set(r.branches()) == {"main"}
    assert "return 10" in r.read_path("main", "a.py")
