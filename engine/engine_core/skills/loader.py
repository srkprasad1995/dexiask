"""
Load skill packs from the mounted /skills volume.

Skill packs are directories under skills_path, each containing a SKILL.md
file. Their contents are appended to the system prompt so Claude is aware
of the available skills for the current role.
"""
from __future__ import annotations

from pathlib import Path

from ..models.events import log


def load_skills(skills_path: str) -> str:
    """
    Walk *skills_path* for ``*/SKILL.md`` files and return a system-prompt addendum.

    Returns an empty string if *skills_path* is empty, missing, or contains no
    ``SKILL.md`` files, so callers can safely concatenate without extra whitespace.
    """
    if not skills_path:
        return ""

    root = Path(skills_path)
    if not root.exists():
        log(f"Skills path not found: {skills_path} (skipping)")
        return ""

    sections: list[str] = []
    for skill_file in sorted(root.rglob("SKILL.md")):
        # Project Mode packs (plan 04) live under <skills>/project/ but are NOT
        # globbed here: they have no role/state filter, so loading them for every
        # conversation would leak all stage/state behaviors at once. ChatService
        # reads the active (stage,state) packs by path and composes them
        # selectively instead. Skip the entire project/ subtree.
        if "project" in skill_file.relative_to(root).parts[:-1]:
            continue
        try:
            content = skill_file.read_text(encoding="utf-8").strip()
            if not content:
                continue
            pack_name = skill_file.parent.name
            sections.append(f"## Skill: {pack_name}\n\n{content}")
            log(f"Loaded skill pack: {pack_name}")
        except OSError as e:
            log(f"Failed to read {skill_file}: {e}")

    if not sections:
        return ""

    return "\n\n---\n\n# Available Skill Packs\n\n" + "\n\n---\n\n".join(sections)
