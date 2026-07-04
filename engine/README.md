# Dexiask engine

An always-on HTTP service that implements the **Agent Job Protocol** and runs the
**Claude Agent SDK** in read-only **ask** mode. A Go backend POSTs a Job to
`POST /v1/jobs` and reads a streaming **NDJSON** event response until a terminal
`result` / `error` event.

This is the open-source engine, trimmed to the single
`ask` role: a read-only assistant that can `Read`/`Glob`/`Grep` a mounted
workspace, use `WebSearch`/`WebFetch`, ask structured `AskChoice` questions, and
reach any indexer/tool MCP servers the Job attaches.

## Layout

```
engine_core/        Runtime-agnostic shared core
  app.py              FastAPI server: /v1/jobs (NDJSON), /v1/jobs/{id}/cancel, /healthz
  runner.py           Composes system prompt + tools + credentials → RunContext
  runtime.py          AgentRuntime ABC + RunContext / BuiltinTools
  models/             Job, protocol events, settings
  capabilities/       workspace (Read/Glob/Grep), interactive (AskChoice), mcp_router
  skills/loader.py    /skills/*/SKILL.md glob → system-prompt addendum
  tools/descriptors.py  Runtime-neutral ToolDescriptor
  observability/      OpenTelemetry (OTLP traces/metrics/logs)
claude_engine/      Claude Agent SDK implementation of AgentRuntime
  runtime.py          RunContext → ClaudeAgentOptions, streams SDK → protocol events
  core/sdk_mapping.py SDK message stream → NDJSON events
  sdk_adapters.py     Descriptors → in-process SDK MCP servers
  models/settings.py  ClaudeSettings (ANTHROPIC_API_KEY, CLAUDE_ENGINE_MODEL)
  server/app.py       Wires ClaudeRuntime into engine_core.create_app (uvicorn target)
```

## Ask-mode tools

The allowed tool set for an ask turn is read-only:

```
{ Read, Glob, Grep, WebSearch, WebFetch, AskChoice }
```

plus any tools exposed by MCP servers the Job attaches in `mcpServers[]`
(e.g. an indexer). There is no `Edit`/`Write`/`Bash`.

## Run it

```bash
# Install (venv recommended)
pip install -e '.[dev]'

# Configure
cp .env.example .env    # set ANTHROPIC_API_KEY (and optionally CLAUDE_ENGINE_MODEL)

# Start the service on 0.0.0.0:8080
ANTHROPIC_API_KEY=sk-ant-... python -m claude_engine

# Health check
curl localhost:8080/healthz     # {"status":"ok"}
```

### Docker

```bash
docker build -t dexiask/engine:dev .
docker run -p 8080:8080 --env-file .env \
  -v "$PWD/workspace:/workspace" -v "$PWD/skills:/skills:ro" \
  dexiask/engine:dev
```

## Credentials

The Claude SDK authenticates with an Anthropic API key resolved in this order:

1. `apiKey` on the Job (per-request, from an orchestrator's UI config), else
2. `ANTHROPIC_API_KEY` in the engine environment (the OSS single-user path).

The same order applies to `baseUrl` / `ANTHROPIC_BASE_URL` (empty ⇒ provider
default). The key is captured into settings at startup and then scrubbed from the
process env so SDK-spawned tool subprocesses can't read it; the model still gets it
via the SDK's `options.env`.

## Endpoints (pinned contract)

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/jobs` | Submit a Job; streams `application/x-ndjson`. `X-Run-Id` header returned. |
| POST | `/v1/jobs/{run_id}/cancel` | Abort an in-progress job. |
| GET  | `/healthz` | Liveness → `{"status":"ok"}`. |

Event type names (`text.*`, `thinking.*`, `tool.*`, `agent.step`, terminal
`result` / `error`) are fixed by [`PROTOCOL.md`](PROTOCOL.md) — the Go backend
re-frames them verbatim.

## Environment

| Variable | Required | Notes |
|---|---|---|
| `ANTHROPIC_API_KEY` | Yes* | *Unless every Job carries its own `apiKey`. |
| `CLAUDE_ENGINE_MODEL` | No | Default model when a Job omits one (default `claude-sonnet-5`). |
| `ANTHROPIC_BASE_URL` | No | Anthropic-compatible gateway base URL. |
| `CLAUDE_ENGINE_SERVER_HOST` / `_PORT` | No | Bind address / port (default `0.0.0.0:8080`). |
| `CLAUDE_ENGINE_MCP_SEARCH_MAX_RESULTS` | No | Max tools returned by the deferred-MCP `search_tools` (default 5). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | OTLP collector base URL; empty disables telemetry. |

## Tests

```bash
pip install -e '.[dev]'
python -m pytest -q
ruff check .
```
