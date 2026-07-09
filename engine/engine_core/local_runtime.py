"""
Local-model agent loop — the no-API-key fallback runtime.

Runs a compact tool-calling loop directly against a local Ollama server's
native ``/api/chat`` instead of driving the full Claude Code CLI. The CLI's
scaffolding (its own system prompt, every MCP tool schema, the skill packs)
adds tens of thousands of prompt tokens per turn — fine for hosted Claude,
but a small local model on laptop CPU spends minutes just *evaluating* that
prompt. This loop keeps the prompt to the Job's role prompt plus a handful of
tools, so small models start answering in seconds.

Kept deliberately small:
  - Tools: the workspace descriptors (Read/Glob/Grep) plus an allowlist of
    remote MCP tools (``settings.local_remote_tools``, e.g. semantic_search)
    resolved via the shared ``RemoteMcpRouter``.
  - No skills addendum, no interactive tools, no deferred-MCP router — a
    small model cannot use them effectively and they bloat the context.
  - Session continuity: the orchestrator sends only the new user message plus
    a sessionId (no history replay), so this runtime persists its own
    user/assistant history as JSON under the session store path.
"""
from __future__ import annotations

import json
import uuid
from pathlib import Path
from typing import Any

import httpx

from .capabilities.mcp_router import RemoteMcpRouter
from .models.events import (
    AgentStepEvent,
    ResultEvent,
    TextDeltaEvent,
    TextStartEvent,
    TextStopEvent,
    ToolInputDoneEvent,
    ToolResultEvent,
    ToolStartEvent,
    Usage,
    log,
)
from .runtime import AgentRuntime, RunContext
from .tools.descriptors import ToolDescriptor, invoke_safely

# Cap on a single tool result fed back into the model, protecting the small
# local context window from one oversized file read or search result.
_MAX_TOOL_RESULT_CHARS = 8_000


def _content_text(payload: dict[str, Any]) -> str:
    """Flatten an SDK-MCP content payload ({"content": [blocks]}) to text."""
    parts = [b.get("text", "") for b in payload.get("content", []) if b.get("type") == "text"]
    text = "\n".join(p for p in parts if p) or "(no content)"
    if len(text) > _MAX_TOOL_RESULT_CHARS:
        text = text[:_MAX_TOOL_RESULT_CHARS] + "\n[... truncated]"
    return text


def _attachment_note(message: Any) -> str:
    """Render a message's attachments as a plain-text note (no vision locally)."""
    notes = [
        f"[Attached file: {att.filename} — read it at {att.path}]"
        for att in message.attachments
    ]
    return "\n".join(notes)


class _SessionStore:
    """User/assistant history persisted per session id (JSON on the FS)."""

    def __init__(self, root: str) -> None:
        self._dir = Path(root) / "local-sessions"

    def load(self, session_id: str) -> list[dict[str, str]]:
        path = self._dir / f"{session_id}.json"
        try:
            return json.loads(path.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            return []

    def save(self, session_id: str, history: list[dict[str, str]]) -> None:
        try:
            self._dir.mkdir(parents=True, exist_ok=True)
            (self._dir / f"{session_id}.json").write_text(
                json.dumps(history), encoding="utf-8"
            )
        except OSError as exc:  # pragma: no cover - defensive
            log(f"local runtime: could not persist session {session_id!r}: {exc}")


class LocalOllamaRuntime(AgentRuntime):
    """Minimal agent loop over a local Ollama server (native /api/chat)."""

    def name(self) -> str:
        return "local"

    def credential_env_names(self) -> tuple[str, str]:
        return ("OLLAMA_API_KEY", "OLLAMA_BASE_URL")

    def supports_interactive(self) -> bool:
        return False

    # ── Tool assembly ──────────────────────────────────────────────────────

    @staticmethod
    def _tool_def(name: str, description: str, schema: dict[str, Any]) -> dict[str, Any]:
        return {
            "type": "function",
            "function": {"name": name, "description": description, "parameters": schema},
        }

    async def _assemble_tools(
        self, ctx: RunContext
    ) -> tuple[list[dict[str, Any]], dict[str, ToolDescriptor], dict[str, str], RemoteMcpRouter | None]:
        """Build the Ollama tool definitions plus dispatch maps.

        Returns (tool_defs, builtin_by_name, remote_tool→server, router).
        """
        defs: list[dict[str, Any]] = []
        builtin: dict[str, ToolDescriptor] = {}
        for d in ctx.builtin_tools.workspace:
            builtin[d.name] = d
            defs.append(self._tool_def(d.name, d.description, d.input_schema))

        remote: dict[str, str] = {}
        router: RemoteMcpRouter | None = None
        allowlist = {
            t.strip() for t in ctx.settings.local_remote_tools.split(",") if t.strip()
        }
        expose_all = "*" in allowlist
        if allowlist and ctx.remote_native:
            router = RemoteMcpRouter(ctx.remote_native, ctx.settings)
            for server in router.server_names:
                try:
                    tools = await router.catalog(server)
                except Exception as e:  # a down server must not sink the turn
                    log(f"local runtime: tool catalog failed for {server!r}: {e}")
                    continue
                for t in tools:
                    if (expose_all or t["name"] in allowlist) and t["name"] not in remote:
                        remote[t["name"]] = server
                        defs.append(
                            self._tool_def(t["name"], t["description"], t["input_schema"])
                        )
        return defs, builtin, remote, router

    # ── The loop ───────────────────────────────────────────────────────────

    async def run(self, ctx: RunContext) -> str | None:
        settings = ctx.settings
        base = settings.local_base_url.rstrip("/")
        session_id = ctx.session_id or str(uuid.uuid4())
        store = _SessionStore(ctx.session_store_path or settings.session_store_path)
        history = store.load(session_id) if ctx.session_id else []

        # The Job's role prompt (+ memory digest) only — deliberately not
        # ctx.system_prompt, which appends the skill packs.
        messages: list[dict[str, Any]] = [
            {"role": "system", "content": ctx.job.system_prompt}
        ]
        messages.extend(history)
        new_turns: list[dict[str, str]] = []
        for m in ctx.messages:
            content = m.content
            note = _attachment_note(m)
            if note:
                content = f"{content}\n{note}".strip()
            entry = {"role": m.role, "content": content}
            messages.append(entry)
            new_turns.append(entry)

        tool_defs, builtin, remote, router = await self._assemble_tools(ctx)
        log(
            f"local runtime: model={ctx.model!r} at {base!r} "
            f"tools={[t['function']['name'] for t in tool_defs]!r}"
        )

        usage = Usage()
        answer_parts: list[str] = []
        turns = 0
        async with httpx.AsyncClient(timeout=settings.local_request_timeout_s) as client:
            for turn in range(1, ctx.max_turns + 1):
                turns = turn
                payload: dict[str, Any] = {
                    "model": ctx.model,
                    "messages": messages,
                    "stream": True,
                    "options": {"num_predict": ctx.max_tokens},
                }
                if tool_defs:
                    payload["tools"] = tool_defs

                content_parts: list[str] = []
                tool_calls: list[dict[str, Any]] = []
                text_id: str | None = None
                async with client.stream("POST", f"{base}/api/chat", json=payload) as resp:
                    resp.raise_for_status()
                    async for line in resp.aiter_lines():
                        if not line.strip():
                            continue
                        chunk = json.loads(line)
                        msg = chunk.get("message") or {}
                        delta = msg.get("content") or ""
                        if delta:
                            if text_id is None:
                                text_id = f"text-{turn}"
                                ctx.emit(TextStartEvent(id=text_id))
                            ctx.emit(TextDeltaEvent(id=text_id, text=delta))
                            content_parts.append(delta)
                        tool_calls.extend(msg.get("tool_calls") or [])
                        if chunk.get("done"):
                            usage.input_tokens += chunk.get("prompt_eval_count", 0)
                            usage.output_tokens += chunk.get("eval_count", 0)
                if text_id is not None:
                    ctx.emit(TextStopEvent(id=text_id))

                content = "".join(content_parts)
                if content:
                    answer_parts.append(content)
                if not tool_calls:
                    break

                assistant_msg: dict[str, Any] = {"role": "assistant", "content": content}
                assistant_msg["tool_calls"] = tool_calls
                messages.append(assistant_msg)
                for i, call in enumerate(tool_calls):
                    fn = call.get("function") or {}
                    name = fn.get("name", "")
                    args = fn.get("arguments") or {}
                    if isinstance(args, str):  # some models emit JSON strings
                        try:
                            args = json.loads(args)
                        except ValueError:
                            args = {}
                    tool_id = f"tool-{turn}-{i}"
                    ctx.emit(ToolStartEvent(id=tool_id, name=name))
                    ctx.emit(ToolInputDoneEvent(id=tool_id, name=name, input=args))
                    ctx.emit(AgentStepEvent(step=turn, tool=name))
                    if name in builtin:
                        result = await invoke_safely(builtin[name], args)
                    elif name in remote and router is not None:
                        result = await router.proxy_call(remote[name], name, args)
                    else:
                        result = ToolDescriptor.text_result(
                            f"Error: unknown tool {name!r}", is_error=True
                        )
                    text = _content_text(result)
                    ctx.emit(ToolResultEvent(id=tool_id, result=text))
                    messages.append({"role": "tool", "tool_name": name, "content": text})

        # Persist only the user/assistant exchange (tool traffic stays
        # per-turn) so future prompts stay small.
        answer = "\n".join(answer_parts)
        store.save(session_id, history + new_turns + [{"role": "assistant", "content": answer}])

        ctx.emit(
            ResultEvent(
                model=ctx.model,
                session_id=session_id,
                usage=usage,
                num_turns=turns,
            )
        )
        return session_id
