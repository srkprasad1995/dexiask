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


def generate_overview(settings: Settings, skeleton: dict[str, Any]) -> str | None:
    if not settings.enable_llm_overview or not settings.anthropic_api_key:
        return None
    try:
        from anthropic import Anthropic
    except Exception:
        return None

    digest = {
        "dirs": skeleton.get("dirs"),
        "entrypoints": skeleton.get("entrypoints"),
        "readme": skeleton.get("readme"),
        "top_symbols": [
            {"name": s["name"], "path": s["path"], "kind": s["kind"]}
            for s in skeleton.get("top_symbols", [])
        ],
    }
    client = Anthropic(api_key=settings.anthropic_api_key)
    msg = client.messages.create(
        model=settings.overview_model,
        max_tokens=1024,
        messages=[
            {
                "role": "user",
                "content": (
                    "Write a concise architecture overview (5-8 sentences) of this codebase "
                    "for an engineer who has never seen it. Cover what it does, the main "
                    "components, and where to start reading. Structure digest:\n\n"
                    + json.dumps(digest, indent=2)
                ),
            }
        ],
    )
    parts = [b.text for b in msg.content if getattr(b, "type", None) == "text"]
    return "\n".join(parts) if parts else None
