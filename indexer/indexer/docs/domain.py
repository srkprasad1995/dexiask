"""
Optional LLM-generated domain-knowledge docs for a repo.

Off by default (``enable_domain_docs``). From the structural skeleton, a single
Anthropic request produces a small set of high-level docs (architecture, modules,
concepts). These are embedded into the repo's Qdrant collection with
``content_type="doc"`` so ``semantic_search`` returns them alongside code. Returns
``[]`` when disabled, unconfigured, or the model output can't be parsed — callers
degrade to code-only search.
"""
from __future__ import annotations

import json
import re
from dataclasses import dataclass
from typing import Any

from ..settings import Settings
from .digest import skeleton_digest
from .prompts import load_prompt

_VALID_CATEGORIES = {"architecture", "module", "concept", "data-flow", "glossary"}


@dataclass(frozen=True)
class DomainDoc:
    title: str
    category: str
    slug: str
    body: str


def _slugify(title: str) -> str:
    s = re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")
    return s or "doc"


def _parse_docs(raw: str) -> list[DomainDoc]:
    """Parse the model's JSON array into DomainDocs, tolerating stray fences."""
    text = raw.strip()
    # Strip a ```json ... ``` fence if the model added one despite instructions.
    fence = re.search(r"\[.*\]", text, re.DOTALL)
    if fence:
        text = fence.group(0)
    try:
        items = json.loads(text)
    except (json.JSONDecodeError, ValueError):
        return []
    if not isinstance(items, list):
        return []

    docs: list[DomainDoc] = []
    seen: set[str] = set()
    for item in items:
        if not isinstance(item, dict):
            continue
        title = str(item.get("title") or "").strip()
        body = str(item.get("body") or "").strip()
        if not title or not body:
            continue
        category = str(item.get("category") or "concept").strip()
        if category not in _VALID_CATEGORIES:
            category = "concept"
        slug = _slugify(title)
        # De-dupe slugs so virtual paths stay unique within a repo.
        base, n = slug, 2
        while slug in seen:
            slug = f"{base}-{n}"
            n += 1
        seen.add(slug)
        docs.append(DomainDoc(title=title, category=category, slug=slug, body=body))
    return docs


def generate_domain_docs(settings: Settings, skeleton: dict[str, Any]) -> list[DomainDoc]:
    if not settings.enable_domain_docs or not settings.anthropic_api_key:
        return []
    try:
        from anthropic import Anthropic
    except Exception:
        return []

    digest = skeleton_digest(skeleton)
    client = Anthropic(api_key=settings.anthropic_api_key)
    msg = client.messages.create(
        model=settings.domain_docs_model,
        max_tokens=4096,
        messages=[
            {
                "role": "user",
                "content": load_prompt("domain_docs").replace("{digest}", json.dumps(digest, indent=2)),
            }
        ],
    )
    parts = [b.text for b in msg.content if getattr(b, "type", None) == "text"]
    return _parse_docs("\n".join(parts)) if parts else []
