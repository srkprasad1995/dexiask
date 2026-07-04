# CLAUDE.md

Guidance for Claude Code when working in the **Dexiask** repository.

## What this is

Dexiask is an open-source, single-user AI Ask platform:
**chat with a Claude agent over a mounted codebase, with semantic code search**. It is
a monorepo of five services wired by one `docker-compose.yml`:

| Dir | Service | Stack |
|---|---|---|
| `backend/` | Chat SSE + attachments + engine bridge + Slack bot + indexer proxy | Go, GORM, Postgres |
| `engine/` | Agent Job Protocol server running the Claude Agent SDK | Python, FastAPI |
| `indexer/` | Semantic + lexical code search, MCP + REST | Python, Qdrant |
| `web/` | Next.js BFF + chat UI + indexer page | TypeScript, React |
| `skills/` | Skill packs mounted into the engine at `/skills` | Markdown |

Each service has its own `README.md`. This file covers cross-service concerns.

## Non-goals (deliberately absent — do not add without discussion)

No user management, auth, workspaces, projects, tasks, runtime config UI, memory, cost
analytics, or multi-engine support. Dexiask is Claude-only and single-tenant by design.
If a change reintroduces any of these, reconsider — it belongs in the upstream platform,
not here.

## Central contracts (change these together)

### Agent Job Protocol (`engine/PROTOCOL.md`)
The Go backend POSTs a Job to the engine's `POST /v1/jobs` and reads a streaming
**NDJSON** event response until a terminal `result`/`error`. The terminal `result` carries
a `sessionId` the backend persists per conversation and sends back next turn so the Claude
SDK resumes natively (no history replay). Event **type names** (`text.*`, `thinking.*`,
`tool.*`, `agent.step`, `result`, `error`) match what the web app expects — the backend
re-frames, it does not translate. Changing an event name means updating the engine, the
Go bridge (`backend/internal/agent`), and `web/src/lib/chat/go-sse-to-ui.ts` together.

### SSE envelope (backend → web → browser)
```
backend POST /v1/chat/stream  → SSE frames (data: <Event JSON>\n\n)
  → web/src/lib/chat/go-sse-to-ui.ts   ← the single translation point
    → AI SDK UI message stream → useChat in the browser
```
`go-sse-to-ui.ts` is the linchpin. Any change to backend SSE event shapes must be
reflected there.

### Shared `/workspace` mount
One host directory (`DEXIASK_WORKSPACE_PATH`) is mounted into the backend (rw — writes
uploaded attachments under `.dexiask/`), the engine (rw — the agent's cwd), and the
indexer (ro — source for its git mirrors). This shared filesystem is the one coupling to
preserve: the backend writes an attachment and the engine reads it back at the same path.

### Indexer wiring
The backend reverse-proxies `/v1/indexer/*` to the indexer's REST control plane
(`DEXIASK_INDEXER_URL`) and injects the indexer's MCP endpoint (`DEXIASK_INDEXER_MCP_URL`)
into every ask Job's `mcpServers[]`, so the agent can call `semantic_search`.

The indexer proxy is transparent (any method+body), so new indexer endpoints work
without backend changes. **Git token**: `PUT/GET /v1/indexer/v1/git-token` provisions a
token the indexer uses to clone private HTTPS repos. It is held server-side, persisted
`0600` on the indexer's data volume, and never returned to the browser (only a
`{configured: bool}` flag). `build_repo_mirror` defaults to `settings.git_token`, so
reindex and the periodic reconcile pick it up automatically.

### Custom MCP servers
User-defined MCP servers are a first-class CRUD entity in the backend
(`model.MCPServer`, `/v1/mcp-servers` handler, GORM-backed). `ChatService` appends every
**enabled** server (mapped to `agent.MCPServerConfig{Name,Type,URL,Headers}`) to each ask
Job's `mcpServers[]`, after the indexer server. Fields are intentionally minimal: name,
type (`http`/`sse`), url, optional headers, enabled — no `deferLoading`/role knobs. Headers
may hold secrets and are stored in Postgres as JSON (plaintext-at-rest, acceptable for this
single-user tool). The web **MCP** page (`/mcp`) manages them via the `/api/mcp/*` BFF proxy.

## Environment

Copy `.env.example` to `.env`. All secrets come from env (`ANTHROPIC_API_KEY`,
`VOYAGE_API_KEY`, `SLACK_*`); nothing is baked into images. Service URLs use
compose-internal DNS (`http://engine:8080`, `http://indexer:8080`, `http://qdrant:6333`,
`postgres:5432`).

## Coding guidelines

- **Clean, modular, readable — this is open source.** Match the surrounding file's style.
  Delete dead code in the same change; no commented-out blocks.
- **Surgical changes.** Touch only what the task needs. Remove imports/vars you orphan.
- **No prompt text in code.** Agent prompts live in embedded `.md` files
  (`backend/internal/agent/prompts/`) and in `skills/*/SKILL.md`, never as string literals.
- **Go:** Clean Architecture (Handler → Service → Repository → GORM). Use
  `go.uber.org/mock/mockgen` for test mocks — never hand-write them. No mock/fake
  fallbacks in runtime code; wire real infra and fail loudly when it's missing.
- **Python:** typed, `ruff`-clean; tests via `pytest`.
- **Web:** typed TS; tests via `vitest` + Testing Library.
- **Tests are mandatory** for every change. Run `make test` (or the per-service target).

## Commands

```bash
make up      # build + start the whole stack
make down    # stop
make logs    # tail all services
make test    # run every test suite
make lint    # vet + ruff + eslint
make clean   # stop and wipe volumes (DB + index)
```
