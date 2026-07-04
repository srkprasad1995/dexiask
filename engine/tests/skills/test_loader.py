"""Tests for skills/loader.py — skill pack discovery."""

from engine_core.skills.loader import load_skills


def test_empty_path_returns_empty():
    assert load_skills("") == ""


def test_missing_path_returns_empty():
    assert load_skills("/nonexistent/path") == ""


def test_no_skill_files(tmp_path):
    assert load_skills(str(tmp_path)) == ""


def test_single_skill_loaded(tmp_path):
    skill_dir = tmp_path / "test-skill"
    skill_dir.mkdir()
    (skill_dir / "SKILL.md").write_text("# Test Skill\nDo things.")

    result = load_skills(str(tmp_path))
    assert "test-skill" in result
    assert "Do things." in result
    assert "Available Skill Packs" in result


def test_multiple_skills_sorted(tmp_path):
    for name in ["zzz-skill", "aaa-skill"]:
        d = tmp_path / name
        d.mkdir()
        (d / "SKILL.md").write_text(f"# {name}")

    result = load_skills(str(tmp_path))
    assert result.index("aaa-skill") < result.index("zzz-skill")


def test_empty_skill_file_skipped(tmp_path):
    skill_dir = tmp_path / "empty-skill"
    skill_dir.mkdir()
    (skill_dir / "SKILL.md").write_text("   \n  ")

    assert load_skills(str(tmp_path)) == ""


def test_unreadable_file_skipped(tmp_path):
    skill_dir = tmp_path / "bad-skill"
    skill_dir.mkdir()
    skill_file = skill_dir / "SKILL.md"
    skill_file.write_text("content")
    skill_file.chmod(0o000)

    try:
        result = load_skills(str(tmp_path))
        # Should not raise; bad file is logged and skipped.
        assert "bad-skill" not in result
    finally:
        skill_file.chmod(0o644)


def test_nested_skill_discovery(tmp_path):
    """SKILL.md files in subdirectories are discovered via rglob."""
    nested = tmp_path / "parent" / "child-skill"
    nested.mkdir(parents=True)
    (nested / "SKILL.md").write_text("Nested content.")

    result = load_skills(str(tmp_path))
    assert "child-skill" in result
    assert "Nested content." in result


def test_project_packs_excluded_from_glob(tmp_path):
    """Project Mode packs under project/ are NOT loaded by the /skills glob
    (plan 04 isolation): ChatService composes them selectively by path."""
    # An ordinary pack IS loaded.
    ok = tmp_path / "regular-skill"
    ok.mkdir()
    (ok / "SKILL.md").write_text("Regular body.")
    # Project per-state packs and the project base-memory pack must be skipped.
    for state in ("grounding", "brainstorming", "documenting", "base-memory"):
        d = tmp_path / "project" / state
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"PROJECT {state} body.")

    result = load_skills(str(tmp_path))
    assert "Regular body." in result
    for state in ("grounding", "brainstorming", "documenting", "base-memory"):
        assert f"PROJECT {state} body." not in result
