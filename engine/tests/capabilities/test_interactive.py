"""Tests for capabilities/interactive.py — AskChoice descriptor."""
import pytest

from engine_core.capabilities.interactive import (
    _ACK,
    ASK_CHOICE_SCHEMA,
    ask_choice,
    build_interactive_descriptors,
)


def test_schema_requires_questions():
    assert ASK_CHOICE_SCHEMA["required"] == ["questions"]
    item = ASK_CHOICE_SCHEMA["properties"]["questions"]["items"]
    assert set(item["required"]) == {"question", "header", "multiSelect", "options"}


def test_build_descriptor_named_ask_choice():
    descs = build_interactive_descriptors()
    assert [d.name for d in descs] == ["AskChoice"]


@pytest.mark.asyncio
async def test_handler_returns_ack_without_blocking():
    result = await ask_choice({"questions": []})
    assert result == {"content": [{"type": "text", "text": _ACK}]}
