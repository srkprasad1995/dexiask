# Dexiask indexer — always-on MCP + REST code-search service.
# Multi-stage: install deps into a venv, then a slim runtime with git.
FROM python:3.12-slim AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY pyproject.toml ./
COPY indexer ./indexer
RUN python -m venv /opt/venv \
    && /opt/venv/bin/pip install --upgrade pip -q \
    && /opt/venv/bin/pip install -q ".[chunking]"

FROM python:3.12-slim AS runtime

# git is required at runtime for mirror/fetch and all read plumbing.
RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /opt/venv /opt/venv
COPY --from=build /app/indexer /app/indexer
COPY repos.yaml /app/repos.yaml

ENV PATH="/opt/venv/bin:$PATH" \
    DEXIASK_SERVER_HOST=0.0.0.0 \
    DEXIASK_SERVER_PORT=8080

WORKDIR /app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=15s \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/healthz')"

# Entry point runs uvicorn on 0.0.0.0:8080 with the background reconcile loop
# enabled (indexes registered repos on startup, reconciles on an interval).
CMD ["python", "-m", "indexer"]
