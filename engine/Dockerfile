# Dexiask engine — Agent Job Protocol
# Long-running HTTP service (FastAPI + Uvicorn) running the Claude Agent SDK.
#
# The SDK bundles the Node Claude Code CLI, so Node.js is required in the image.
#   docker build -t dexiask/engine:dev .

# ── Stage 1: deps (Node + Python deps) ──────────────────────────────────────
FROM python:3.12-slim AS deps
WORKDIR /build

# Node.js is required by the Claude Agent SDK (it spawns the claude-code CLI).
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl gnupg \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install the engine package + its dependencies (incl. the Claude Agent SDK).
COPY pyproject.toml ./
COPY engine_core/ ./engine_core/
COPY claude_engine/ ./claude_engine/
RUN pip install --no-cache-dir .

# ── Stage 2: final image ────────────────────────────────────────────────────
FROM python:3.12-slim
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Copy Node + npm and the installed Python site-packages from the deps stage.
COPY --from=deps /usr/bin/node /usr/bin/node
COPY --from=deps /usr/lib/node_modules /usr/lib/node_modules
RUN ln -sf /usr/lib/node_modules/npm/bin/npm-cli.js /usr/bin/npm 2>/dev/null || true
COPY --from=deps /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=deps /usr/local/bin /usr/local/bin

# Mount points:
#   /workspace    — source-code workspace (Read/Glob/Grep tools)
#   /skills       — read-only skill-pack mount (SKILL.md discovery)
#   /root/.claude — SDK session store (mount a volume for persistence)
VOLUME ["/workspace", "/skills", "/root/.claude"]

EXPOSE 8080
ENV CLAUDE_ENGINE_SERVER_HOST=0.0.0.0 CLAUDE_ENGINE_SERVER_PORT=8080
ENTRYPOINT ["python", "-m", "claude_engine"]
