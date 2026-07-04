"""
Entry point: ``python -m claude_engine``

Starts the Uvicorn HTTP server using settings from environment variables.
The server exposes the Agent Job Protocol API at ``POST /v1/jobs``.

For local development:

    ANTHROPIC_API_KEY=sk-... python -m claude_engine

Environment variables that control the server:
    CLAUDE_ENGINE_SERVER_HOST  — bind address (default: 0.0.0.0)
    CLAUDE_ENGINE_SERVER_PORT  — bind port    (default: 8080)
    CLAUDE_ENGINE_MODEL        — default model when the Job omits one
"""
import uvicorn

from engine_core import observability as obs

from .models.settings import get_settings
from .server.app import create_app


def main() -> None:
    cfg = get_settings()
    # Install OTel providers + logging before building the app so create_app's
    # FastAPI instrumentation binds to the real tracer/propagator.
    obs.init_observability(cfg)
    app = create_app(settings=cfg)
    uvicorn.run(
        app,
        host=cfg.server_host,
        port=cfg.server_port,
        log_level="info",
    )


if __name__ == "__main__":
    main()
