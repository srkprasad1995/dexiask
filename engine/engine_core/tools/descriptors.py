"""
Runtime-neutral tool descriptor.

A ``ToolDescriptor`` is the runtime-neutral representation of a tool the shared
core wants the model to be able to call: a name, a description, a JSON Schema for
its input, and an ``async`` handler. The Claude runtime adapts these into its own
tool mechanism, wrapping them as ``create_sdk_mcp_server`` ``@tool`` functions
(see ``claude_engine.sdk_adapters``).

The handler returns the SDK-MCP content shape
``{"content": [{"type": "text", "text": ...}], "is_error"?: bool}`` so the adapter
can unwrap the text uniformly.
"""
from __future__ import annotations

from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Any

from ..models.events import log

ToolHandler = Callable[[dict[str, Any]], Awaitable[dict[str, Any]]]


@dataclass(frozen=True)
class ToolDescriptor:
    """One callable tool, independent of any agent runtime."""

    name: str
    description: str
    input_schema: dict[str, Any]
    handler: ToolHandler

    @staticmethod
    def text_result(text: str, is_error: bool = False) -> dict[str, Any]:
        """Build the standard SDK-MCP content payload from a plain string."""
        out: dict[str, Any] = {"content": [{"type": "text", "text": text}]}
        if is_error:
            out["is_error"] = True
        return out


async def invoke_safely(descriptor: ToolDescriptor, args: dict[str, Any]) -> dict[str, Any]:
    """Invoke a descriptor's handler, never letting a failure escape.

    A raised exception is converted into an ``is_error`` tool result so the model
    receives the failure as a readable message it can recover from — instead of
    the exception bubbling up and aborting the entire turn (which surfaces to the
    user as a hard error / infinite spinner). Every engine adapter routes tool
    calls through here so the behaviour is uniform across runtimes.
    """
    try:
        return await descriptor.handler(args)
    except Exception as exc:  # noqa: BLE001 - any tool failure must stay local
        log(f"tool {descriptor.name!r} failed: {exc}")
        return ToolDescriptor.text_result(
            f"Error: tool {descriptor.name!r} failed: {exc}", is_error=True
        )
