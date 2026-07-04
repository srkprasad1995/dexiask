"""Entry point: ``python -m indexer`` starts the HTTP server."""
from __future__ import annotations

import uvicorn

from .app import create_app
from .settings import get_settings


def main() -> None:
    settings = get_settings()
    uvicorn.run(
        create_app(run_scheduler=True), host=settings.server_host, port=settings.server_port
    )


if __name__ == "__main__":
    main()
