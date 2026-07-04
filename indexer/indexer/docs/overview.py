"""
Optional one-shot LLM overview of a repo, generated from the structural skeleton.

Off by default (``enable_llm_overview``). Calls Anthropic directly — a single
summarization request, no session/agent coupling. Returns None when disabled or
no key is set, so callers fall back to the deterministic skeleton.
"""
from __future__ import annotations

import json
from typing import Any

from ..settings import Settings
from .digest import skeleton_digest
from .prompts import load_prompt


def generate_overview(settings: Settings, skeleton: dict[str, Any]) -> str | None:
    if not settings.enable_llm_overview or not settings.anthropic_api_key:
        return None
    try:
        from anthropic import Anthropic
    except Exception:
        return None

    digest = skeleton_digest(skeleton)
    client = Anthropic(api_key=settings.anthropic_api_key)
    msg = client.messages.create(
        model=settings.overview_model,
        max_tokens=1024,
        messages=[
            {
                "role": "user",
                "content": load_prompt("overview").replace("{digest}", json.dumps(digest, indent=2)),
            }
        ],
    )
    parts = [b.text for b in msg.content if getattr(b, "type", None) == "text"]
    return "\n".join(parts) if parts else None
