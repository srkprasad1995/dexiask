"""
Runtime-agnostic run orchestration.

Composes the system prompt (skills), resolves provider credentials, builds the
shared built-in capabilities, splits remote MCP servers into native/deferred, and
assembles a ``RunContext`` — then delegates the actual model loop to a concrete
``AgentRuntime``.

Everything provider-specific (SDK options, stream translation, attachment
encoding) lives in the runtime; everything shared lives here.
"""
from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

from .capabilities.interactive import build_interactive_descriptors
from .capabilities.mcp_router import RemoteMcpRouter, build_mcp_router_descriptors
from .capabilities.workspace import WORKSPACE_TOOL_NAMES, build_workspace_descriptors
from .models.events import ErrorEvent, _BaseEvent, log
from .models.job import Job, PermissionMode
from .models.settings import BaseEngineSettings
from .runtime import AgentRuntime, BuiltinTools, RemoteMcp, RunContext
from .skills.loader import load_skills


def _to_remote(servers: list) -> list[RemoteMcp]:
    return [
        RemoteMcp(
            name=s.name,
            type=s.type,
            url=s.url,
            description=s.description,
            headers=s.headers or {},
            defer_loading=s.defer_loading,
        )
        for s in servers
    ]


async def run(
    job: Job,
    runtime: AgentRuntime,
    emit_fn: Callable[[_BaseEvent], None],
    settings: BaseEngineSettings,
) -> str | None:
    """Compose the run context and dispatch to *runtime*.

    Emits an ``ErrorEvent`` and returns ``None`` on unrecoverable setup errors.
    """
    model = job.resolved_model(settings.default_model)
    log(
        f"Starting. runtime={runtime.name()!r} role={job.role.value!r} model={model!r} "
        f"allowed_tools={job.allowed_tools!r} permission_mode={job.permission_mode.value!r} "
        f"session_id={job.session_id!r}"
    )

    # ── Resolve provider credentials ───────────────────────────────────────
    # Credentials come from the Job when the orchestrator supplies per-workspace
    # UI-configured settings; otherwise the engine falls back to its own env key
    # (ANTHROPIC_API_KEY / ANTHROPIC_BASE_URL, captured into Settings at startup).
    # This env fallback is what makes the OSS single-user build work with just an
    # API key in the environment. An empty base URL means "use the provider default".
    key_var, base_var = runtime.credential_env_names()
    effective_api_key = job.api_key or settings.engine_api_key
    effective_base_url = job.base_url or settings.engine_base_url
    if not effective_api_key:
        emit_fn(
            ErrorEvent(
                message=(
                    f"No API credentials available. Set {key_var} in the engine "
                    "environment or supply an apiKey on the Job."
                )
            )
        )
        return None
    provider_env: dict[str, str] = {key_var: effective_api_key}
    if effective_base_url:
        provider_env[base_var] = effective_base_url

    # ── System prompt: base + skills ───────────────────────────────────────
    skills_addendum = load_skills(job.skills_path)
    system_prompt = job.system_prompt + skills_addendum

    # ── Attachment force-enable ────────────────────────────────────────────
    # Non-image files are delivered as /workspace paths, so the Read tool must be
    # available even if the role didn't request it.
    has_attachments = any(m.attachments for m in job.messages)
    effective_tools = list(job.allowed_tools)
    effective_mode = job.permission_mode
    if has_attachments:
        for t in ("Read", "Glob", "Grep"):
            if t not in effective_tools:
                effective_tools.append(t)
        if effective_mode == PermissionMode.DEFAULT:
            effective_mode = PermissionMode.DONT_ASK

    # ── Built-in capabilities (gated by permission mode) ───────────────────
    builtin = BuiltinTools()
    if effective_mode != PermissionMode.DEFAULT:
        ws_tools = [t for t in effective_tools if t in WORKSPACE_TOOL_NAMES]
        if ws_tools:
            builtin.workspace = build_workspace_descriptors(
                job.workspace_path, ws_tools, settings
            )
        # AskChoice is registered only when its name appears in allowedTools
        # (and the runtime supports interactive tools).
        if "AskChoice" in effective_tools and runtime.supports_interactive():
            builtin.interactive = build_interactive_descriptors()

    # ── Remote MCP servers: native vs deferred ─────────────────────────────
    native = [s for s in job.mcp_servers if not s.defer_loading]
    deferred = [s for s in job.mcp_servers if s.defer_loading]
    if deferred:
        router = RemoteMcpRouter(deferred, settings)
        builtin.mcp_router = build_mcp_router_descriptors(router)
        listing = "\n".join(f"- {s.name} — {s.description.strip()}" for s in deferred)
        system_prompt += (
            "\n\n## Additional MCP servers (tools load on demand)\n"
            "These servers are available; their tools are NOT preloaded. Pick the "
            "relevant server by id, then call `search_tools` (optionally with "
            "`server=<id>`) to find its tools, then `call_tool` with the `server` and "
            "`name` from the result and arguments matching its `input_schema`. "
            "`list_mcp_servers` returns this same list.\n"
            f"{listing}"
        )
        log(f"Deferred MCP servers via tool-search router: {router.server_names!r}")

    # ── Ensure the workspace cwd exists (per-workspace, may be first turn) ──
    try:
        Path(job.workspace_path).mkdir(parents=True, exist_ok=True)
    except OSError as exc:  # pragma: no cover - defensive
        log(f"Warning: could not create workspace path {job.workspace_path!r}: {exc}")

    ctx = RunContext(
        job=job,
        system_prompt=system_prompt,
        model=model,
        workspace_path=job.workspace_path,
        session_id=job.session_id,
        session_store_path=job.session_store_path,
        builtin_tools=builtin,
        remote_native=_to_remote(native),
        remote_deferred=_to_remote(deferred),
        provider_env=provider_env,
        permission_mode=effective_mode.value,
        requested_tools=effective_tools,
        settings=settings,
        emit=emit_fn,
    )

    try:
        return await runtime.run(ctx)
    except Exception as e:  # pragma: no cover - defensive top-level guard
        log(f"Fatal: {e}")
        emit_fn(ErrorEvent(message=str(e)))
        return None
