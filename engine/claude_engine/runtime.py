"""
ClaudeRuntime — the Claude Agent SDK implementation of ``AgentRuntime``.

The shared ``engine_core.runner`` composes the system prompt (skills), the
built-in capabilities, attachment force-enable and the remote-MCP split, then
hands a ``RunContext`` here. This runtime turns that context into
``ClaudeAgentOptions``, attaches the capabilities as in-process SDK MCP servers,
calls ``query()`` and streams the SDK messages back as protocol events via
``sdk_mapping``.
"""
from __future__ import annotations

import base64
from collections.abc import AsyncIterator
from pathlib import Path

from claude_agent_sdk import ClaudeAgentOptions, query

from engine_core.models.events import log
from engine_core.runtime import AgentRuntime, RunContext

from .core.sdk_mapping import process_sdk_messages
from .sdk_adapters import (
    build_remote_mcp,
    descriptors_to_server,
    sdk_allowed_names,
)

_MAX_INLINE_IMAGE_BYTES = 5 * 1024 * 1024  # 5 MB cap for base64 inlining

# Built-in SDK tools allow-listed by their plain name (no MCP namespace).
# The ask role is read-only, so only the web tools are exposed here.
_BUILTIN_TOOLS = {"WebSearch", "WebFetch"}
# Built-in Claude Code tool we shadow with our MCP AskChoice (can't run headlessly).
_DISALLOWED_BUILTINS = ["AskUserQuestion"]


def _build_message_content(message, workspace_root: str) -> str | list[dict]:
    """Build Anthropic content for a single message (Claude-specific encoding).

    No attachments → the content string (fast path). Otherwise a content-block
    list: images <= 5 MB as base64 blocks, images > 5 MB or non-images as text
    path references the Read tool can open.
    """
    if not message.attachments:
        return message.content

    blocks: list[dict] = []
    if message.content.strip():
        blocks.append({"type": "text", "text": message.content})

    root = Path(workspace_root).resolve()

    for att in message.attachments:
        if att.kind == "image":
            att_path = Path(att.path)
            try:
                resolved = att_path.resolve()
                resolved.relative_to(root)
                size = resolved.stat().st_size
                if size <= _MAX_INLINE_IMAGE_BYTES:
                    data = base64.standard_b64encode(resolved.read_bytes()).decode()
                    blocks.append({
                        "type": "image",
                        "source": {
                            "type": "base64",
                            "media_type": att.media_type,
                            "data": data,
                        },
                    })
                else:
                    blocks.append({
                        "type": "text",
                        "text": f"[Image too large to inline ({size // 1024} KB) — read it at {att.path}]",
                    })
            except (ValueError, OSError) as exc:
                blocks.append({
                    "type": "text",
                    "text": f"[Could not read image {att.filename}: {exc}]",
                })
        else:
            blocks.append({
                "type": "text",
                "text": f"[Attached file: {att.filename} — read it at {att.path}]",
            })

    if not blocks:
        blocks.append({"type": "text", "text": "(empty message)"})

    return blocks


class ClaudeRuntime(AgentRuntime):
    def name(self) -> str:
        return "claude"

    def credential_env_names(self) -> tuple[str, str]:
        return ("ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL")

    def supports_thinking(self) -> bool:
        return True

    async def run(self, ctx: RunContext) -> str | None:
        bt = ctx.builtin_tools
        mcp_servers: dict = {}
        sdk_allowed: list[str] = []
        disallowed_tools: list[str] = []

        # ── Built-in capabilities as in-process SDK MCP servers ────────────
        if bt.workspace:
            mcp_servers["workspace"] = descriptors_to_server("workspace", bt.workspace)
            sdk_allowed.extend(sdk_allowed_names("workspace", bt.workspace))
        if bt.interactive:
            mcp_servers["interactive"] = descriptors_to_server("interactive", bt.interactive)
            sdk_allowed.extend(sdk_allowed_names("interactive", bt.interactive))
            disallowed_tools.extend(_DISALLOWED_BUILTINS)
        if bt.mcp_router:
            mcp_servers["mcp_router"] = descriptors_to_server("mcp_router", bt.mcp_router)
            sdk_allowed.extend(sdk_allowed_names("mcp_router", bt.mcp_router))

        # ── Built-in SDK tools (WebSearch/WebFetch) by plain name ──────────
        sdk_allowed.extend(t for t in ctx.requested_tools if t in _BUILTIN_TOOLS)

        # ── Native remote MCP servers (deferred already wired into router) ─
        remote_servers, remote_allow, _ = build_remote_mcp(
            ctx.remote_native, reserved_names=set(mcp_servers)
        )
        mcp_servers.update(remote_servers)
        sdk_allowed.extend(remote_allow)
        if remote_servers:
            log(f"Attached native MCP servers: {list(remote_servers)!r}")
        if sdk_allowed:
            log(f"Tools enabled: {sdk_allowed!r}")

        # ── SDK env (provider creds + per-request output cap) ──────────────
        sdk_env = dict(ctx.provider_env)
        sdk_env["CLAUDE_CODE_MAX_OUTPUT_TOKENS"] = str(ctx.max_tokens)
        # Redirect the SDK session/transcript store to a per-conversation directory
        # when the orchestrator supplies one; otherwise keep the default HOME
        # (~/.claude), so pre-existing conversations still resume.
        session_store = getattr(ctx, "session_store_path", None)
        if session_store:
            Path(session_store).mkdir(parents=True, exist_ok=True)
            sdk_env["CLAUDE_CONFIG_DIR"] = session_store

        options_kwargs: dict = {
            "model": ctx.model,
            "max_turns": ctx.max_turns,
            "include_partial_messages": True,
            "permission_mode": ctx.permission_mode,
            "mcp_servers": mcp_servers,
            "allowed_tools": sdk_allowed,
            "cwd": ctx.workspace_path,
            "env": sdk_env,
        }
        if disallowed_tools:
            options_kwargs["disallowed_tools"] = disallowed_tools
        if ctx.effort:
            options_kwargs["effort"] = ctx.effort
        if ctx.thinking_enabled:
            options_kwargs["thinking"] = {
                "type": "adaptive",
                "display": ctx.thinking_display,
            }
        if ctx.fallback_model:
            options_kwargs["fallback_model"] = ctx.fallback_model
        if ctx.betas:
            options_kwargs["betas"] = ctx.betas
        if ctx.system_prompt:
            options_kwargs["system_prompt"] = ctx.system_prompt
        if ctx.session_id:
            options_kwargs["resume"] = ctx.session_id
            log(f"Resuming session {ctx.session_id!r}")

        options = ClaudeAgentOptions(**options_kwargs)

        # ── Build the prompt (string fast-path, or streaming-input for
        #    attachments). A bare content-block list is NOT a valid SDK prompt. ─
        prompt: str | AsyncIterator[dict]
        messages = ctx.job.messages
        if ctx.has_attachments:
            last_content = _build_message_content(messages[-1], ctx.workspace_path)

            async def _attachment_stream() -> AsyncIterator[dict]:
                yield {
                    "type": "user",
                    "message": {"role": "user", "content": last_content},
                    "parent_tool_use_id": None,
                    "session_id": ctx.session_id or "default",
                }

            prompt = _attachment_stream()
        elif len(messages) == 1:
            prompt = messages[0].content
        else:
            prompt = messages[-1].content

        return await process_sdk_messages(
            query(prompt=prompt, options=options),
            emit_fn=ctx.emit,
            fallback_model=ctx.model,
        )
