"""
Engine HTTP server — Agent Job Protocol (runtime-agnostic).

A long-running HTTP service. ``create_app(runtime, settings)`` wires the
streaming job endpoint to a concrete ``AgentRuntime``.

  POST /v1/jobs                  Body: Job JSON; streams NDJSON events.
  POST /v1/jobs/{run_id}/cancel  Cancel an in-progress job.
  GET  /healthz                  {"status": "ok"}.
"""
from __future__ import annotations

import asyncio
import os
import time
import uuid
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import StreamingResponse

from . import observability as obs
from .models.events import ErrorEvent, ResultEvent, _BaseEvent, log
from .models.job import Job
from .models.settings import BaseEngineSettings, default_settings
from .runner import run as run_job
from .runtime import AgentRuntime

# Active jobs: run_id → asyncio.Task
_active_jobs: dict[str, asyncio.Task] = {}


@asynccontextmanager
async def _lifespan(app: FastAPI) -> AsyncIterator[None]:
    cfg: BaseEngineSettings = app.state.settings
    runtime: AgentRuntime = app.state.runtime
    log(f"{runtime.name()} engine HTTP server starting on {cfg.server_host}:{cfg.server_port}")
    yield
    for task in list(_active_jobs.values()):
        task.cancel()
    if _active_jobs:
        await asyncio.gather(*_active_jobs.values(), return_exceptions=True)
    obs.shutdown_observability()
    log(f"{runtime.name()} engine HTTP server stopped.")


def _scrub_platform_env(runtime: AgentRuntime) -> None:
    """Remove the provider credential vars from ``os.environ`` at startup.

    ``Settings`` has already captured the API key/base URL (see ``get_settings``,
    which runs before this), so the model still authenticates via the per-Job
    ``options.env``. Popping the vars from the process env means SDK-spawned tool
    subprocesses (e.g. a Bash ``env``) can't inherit the raw key.
    """
    key_var, base_var = runtime.credential_env_names()
    for name in (key_var, base_var):
        os.environ.pop(name, None)


def create_app(
    runtime: AgentRuntime,
    settings: BaseEngineSettings | None = None,
) -> FastAPI:
    """Create and configure the FastAPI application for *runtime*."""
    cfg = settings or default_settings()
    # Settings have captured any credentials they need; scrub the copies from the
    # process env so tool subprocesses can't inherit them.
    _scrub_platform_env(runtime)

    app = FastAPI(
        title=f"Dexiask {runtime.name().title()} Engine",
        description="Agent Job Protocol — HTTP streaming NDJSON",
        version="1.0.0",
        lifespan=_lifespan,
    )
    app.state.settings = cfg
    app.state.runtime = runtime

    obs.init_observability(cfg, engine_name=runtime.name())
    obs.instrument_fastapi(app)

    @app.get("/healthz", tags=["ops"])
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.post("/v1/jobs", tags=["jobs"])
    async def submit_job(request: Request) -> StreamingResponse:
        body = await request.json()
        try:
            job = Job.model_validate(body)
        except Exception as e:
            raise HTTPException(status_code=422, detail=str(e))

        run_id = str(uuid.uuid4())
        log(f"Job {run_id} accepted. role={job.role.value!r}")

        queue: asyncio.Queue[_BaseEvent | None] = asyncio.Queue()
        state = {"error": False}

        def emit_fn(event: _BaseEvent) -> None:
            if isinstance(event, ErrorEvent):
                state["error"] = True
            queue.put_nowait(event)

        last_user_content = job.messages[-1].content if job.messages else ""

        async def _run_agent() -> None:
            start = time.perf_counter()
            with obs.get_tracer().start_as_current_span("job") as span:
                span.set_attribute("dexiask.run_id", run_id)
                span.set_attribute("dexiask.role", job.role.value)
                span.set_attribute("dexiask.runtime", runtime.name())
                span.set_attribute("gen_ai.request.model", job.resolved_model(cfg.default_model))
                if job.session_id:
                    span.set_attribute("dexiask.session_id", job.session_id)
                try:
                    await run_job(job, runtime, emit_fn, cfg)
                except Exception as e:
                    state["error"] = True
                    span.record_exception(e)
                    span.set_status(obs.Status(obs.StatusCode.ERROR))
                    log(f"Agent task unhandled error: {e}")
                    queue.put_nowait(ErrorEvent(message=str(e)))
                finally:
                    is_err = state["error"]
                    if is_err:
                        span.set_status(obs.Status(obs.StatusCode.ERROR))
                    obs.set_span_content(span, "gen_ai.prompt", last_user_content, is_err)
                    obs.record_job("error" if is_err else "success", time.perf_counter() - start)
                    queue.put_nowait(None)  # Sentinel: stream done.
                    _active_jobs.pop(run_id, None)

        task = asyncio.create_task(_run_agent(), name=f"job-{run_id}")
        _active_jobs[run_id] = task

        async def _event_stream() -> AsyncIterator[str]:
            try:
                while True:
                    event = await queue.get()
                    if event is None:
                        break
                    yield event.to_ndjson() + "\n"
                    if isinstance(event, (ResultEvent, ErrorEvent)):
                        break
            except asyncio.CancelledError:
                task.cancel()
                raise

        return StreamingResponse(
            _event_stream(),
            media_type="application/x-ndjson",
            headers={"X-Run-Id": run_id},
        )

    @app.post("/v1/jobs/{run_id}/cancel", tags=["jobs"])
    async def cancel_job(run_id: str) -> dict[str, str]:
        task = _active_jobs.get(run_id)
        if task is None:
            raise HTTPException(status_code=404, detail=f"Job {run_id!r} not found.")
        if not task.done():
            task.cancel()
            log(f"Job {run_id} cancel requested.")
        return {"status": "cancelled", "run_id": run_id}

    return app
