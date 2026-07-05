# CLAUDE.md

Guidance for Claude Code when working in the **Dexiask** repository.

## What this is

Dexiask is an open-source, multi-user AI Ask platform:
**chat with a Claude agent over a mounted codebase, with semantic code search**. Users
sign in with GitHub; every product row is scoped to the resolved GitHub user id. It is
a monorepo of six services wired by one `docker-compose.yml`:

| Dir | Service | Stack |
|---|---|---|
| `backend/` | Chat SSE + attachments + engine bridge + Slack bot + GitHub-OAuth auth + indexer/memory proxy | Go, GORM, Postgres |
| `engine/` | Agent Job Protocol server running the Claude Agent SDK | Python, FastAPI |
| `indexer/` | Semantic + lexical code search + generated domain docs, MCP + REST | Python, Qdrant |
| `memory/` | FS-backed user/repo/global memory + dream consolidation, MCP + REST | Go |
| `web/` | Next.js BFF + login + chat UI + indexer/memory pages | TypeScript, React |
| `skills/` | Skill packs mounted into the engine at `/skills` | Markdown |

Each service has its own `README.md`. This file covers cross-service concerns.

## Scope & non-goals

Dexiask is **multi-user but single-workspace**: one mounted codebase == one workspace, so
`WorkspaceID` is fixed (`config.FixedWorkspaceID = "dexiask"`) while `UserID` is the real
GitHub user id. When no GitHub OAuth app is configured the backend runs in **dev-fallback**
mode (single fixed dev user, no login) so `docker compose up` works with zero config.

Still deliberately absent (do not add without discussion): teams/orgs, multiple workspaces,
projects, tasks, runtime config UI, cost analytics, or multi-engine support. Dexiask is
Claude-only. If a change reintroduces any of these, reconsider — it belongs in the upstream
platform, not here.

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

### Auth + roles (`backend/internal/auth`)
Auth lives entirely in the backend. `auth.Authenticator.Middleware` verifies the signed
session cookie → loads session + user → decrypts the user's GitHub token → injects a
`Principal{UserID, Login, Role, GitHubToken}`. Handlers read it via `requirePrincipal` /
`requireAdmin` and stamp `p.UserID`; **no `config.FixedUserID` on the hot paths** except the
dev-fallback principal.
- **Login methods**: primary is **token login** (`POST /v1/auth/token-login` with a GitHub
  PAT — validated via the GitHub API, stored AES-GCM encrypted). GitHub **OAuth** is
  optional on top (`/v1/auth/login|callback`, enabled when `DEXIASK_GITHUB_CLIENT_ID/SECRET`
  are set). `RequireAuth` is true when the session infra (`DEXIASK_SESSION_SECRET` +
  `DEXIASK_TOKEN_ENC_KEY`) is configured **or** OAuth is; otherwise the middleware injects
  the fixed dev **admin** user (zero-config).
- **Roles**: `admin` / `member` (`model.User.Role`). The **first** user to sign in bootstraps
  as admin; everyone else must hold an admin-created **invite** (`model.Invite`, keyed by
  GitHub login, consumed on first login) or login is refused (403). Admin-only surface,
  gated by `requireAdmin`: `/v1/mcp-servers/*`, indexer **mutations** (reindex/register/
  git-token — reads are open to members), memory **mutations** (`/v1/memory/consolidate` +
  entry edits), and `/v1/invites` + `/v1/users`. `GET /v1/auth/config` advertises which
  login methods are enabled.
- **MCP servers are workspace-wide** (admin-managed, injected into every user's turn) — not
  per-user. **Indexing uses one central git token** set by an admin (the indexer's global
  token via `PUT /v1/indexer/v1/git-token`); the backend no longer injects per-user tokens.
- Web: `/login` (token form + optional OAuth button, driven by `/api/auth/config`), the
  `/admin` Team page (invites + roster), and admin-only nav/actions gated by `useIsAdmin`.

### Indexer wiring
The backend reverse-proxies `/v1/indexer/*` to the indexer's REST control plane
(`DEXIASK_INDEXER_URL`) and injects the indexer's MCP endpoint (`DEXIASK_INDEXER_MCP_URL`)
into every ask Job's `mcpServers[]`, so the agent can call `semantic_search`.

The indexer proxy is transparent (any method+body), so new indexer endpoints work without
backend changes; mutating ops are **admin-only** (see Auth). **Central git token**: indexing
uses one shared token an admin sets via `PUT /v1/indexer/v1/git-token` (the indexer's global
`settings.git_token`, persisted `0600`, never returned to the browser). **Per-user repo
gating (enforced in the indexer)**: when `DEXIASK_INTERNAL_TOKEN` is set, the backend forwards
the caller's identity to the indexer — `X-Internal-Token` (the shared secret), plus
`X-Repo-Access: all` for admins or `X-User-Token: <their GitHub token>` for members (the
backend strips any client-supplied copies). The **indexer** (`access.py` + `github.py`)
validates each member's token against every repo via the GitHub API itself (cached), so a
forged header or a direct hit on the indexer port only ever reveals what the caller's own
token grants; the "unrestricted" admin path is honoured only with a valid secret. Members see
only GitHub-verified repos; local/non-GitHub repos are admin-only. Enforced across
`semantic_search`, repo-scoped tools, and the repo/status listings.  **Domain docs**: when `DEXIASK_ENABLE_DOMAIN_DOCS` is set, each index
pass has an LLM generate architecture/module/concept docs, embedded into Qdrant with
`content_type="doc"` so `semantic_search` returns them alongside code (the `code_only`
filter never drops docs); browsable via `GET /v1/indexer/v1/docs/{repo}` and the web
indexer "Docs" tab. See `indexer/indexer/docs/`.

### Memory service (`memory/` ↔ `backend` ↔ engine)
`memory/` is a standalone Go service (FS-backed, no DB, on the `memory-data` volume). It
owns user/repo/global memory and exposes it as an **MCP server** (`memory_view` /
`memory_write` / `memory_search`) + a small REST surface (`/v1/memory/*`, `/v1/memory/digest`,
`/v1/consolidate`). Four cross-service contracts must change together:
- **engine → memory MCP** (`DEXIASK_MEMORY_MCP_URL`): the backend appends it per ask turn
  with headers `X-Workspace-Id`/`X-User-Id`/`X-Role: ask`/`X-Writable-Scopes: user,repo`
  (built per turn because the headers are per-user). Chat roles may only `observe`.
- **backend → memory REST** (`DEXIASK_MEMORY_URL`): `memory_handler.go` reverse-proxies
  `/v1/memory/*`, injecting the fixed workspace + authenticated user id.
- **digest injection**: at job build the backend fetches `GET /v1/memory/digest` per user
  (best-effort, `backend/internal/memory/client.go`) and appends the `## Memory` block to
  the ask system prompt.
- **dream judge**: the memory service's flock-guarded scheduler builds a "pending working
  memory" context and submits a job to the engine (`Role:"ask"` — the engine's only role;
  the dream distinction rides `X-Role: dream` on the memory MCP + the `dream.md` prompt).
  The engine's LLM curates memory back through the memory MCP (`create`/`update`/`delete`/
  `clear_working`). The web **Memory** page (`/memory`) browses scopes + triggers a manual
  consolidation via the `/api/memory/*` BFF proxy.

### Custom MCP servers
User-defined MCP servers are a first-class CRUD entity in the backend
(`model.MCPServer`, `/v1/mcp-servers` handler, GORM-backed), **scoped per user** — a
server's `Headers` may hold secrets, so one user's servers are never injected into another
user's turn. `ChatService` appends every **enabled** server for the caller (mapped to
`agent.MCPServerConfig{Name,Type,URL,Headers}`) to each ask Job's `mcpServers[]`, after the
indexer + memory servers. Fields are intentionally minimal: name, type (`http`/`sse`), url,
optional headers, enabled — no `deferLoading`/role knobs. The web **MCP** page (`/mcp`)
manages them via the `/api/mcp/*` BFF proxy.

## Environment

Copy `.env.example` to `.env`. All secrets come from env (`ANTHROPIC_API_KEY`,
`VOYAGE_API_KEY`, `SLACK_*`); nothing is baked into images. Service URLs use
compose-internal DNS (`http://engine:8080`, `http://indexer:8080`, `http://memory:8080`,
`http://qdrant:6333`, `postgres:5432`).

Feature-gating env (all optional; each degrades gracefully when unset):
- **Auth**: `DEXIASK_GITHUB_CLIENT_ID`/`_SECRET` (empty → dev-fallback, no login),
  `DEXIASK_OAUTH_CALLBACK_URL`, `DEXIASK_WEB_BASE_URL`, `DEXIASK_SESSION_SECRET`,
  `DEXIASK_TOKEN_ENC_KEY` (hex AES key), and web `AUTH_ENABLED` (mirrors the login gate).
- **Domain docs**: `DEXIASK_ENABLE_DOMAIN_DOCS`, `DEXIASK_DOMAIN_DOCS_MODEL`,
  `DEXIASK_ANTHROPIC_API_KEY` (indexer service).
- **Memory**: `DEXIASK_MEMORY_URL`/`_MCP_URL` (backend → memory), `DEXIASK_DREAM_MODEL`,
  `DEXIASK_DREAM_INTERVAL` (0 disables the dream loop).

## Coding guidelines

- **Clean, modular, readable — this is open source.** Match the surrounding file's style.
  Delete dead code in the same change; no commented-out blocks.
- **Surgical changes.** Touch only what the task needs. Remove imports/vars you orphan.
- **No prompt text in code.** Agent/LLM prompts live in `.md` files loaded at runtime —
  `backend/internal/agent/prompts/`, `skills/*/SKILL.md`, `indexer/indexer/docs/prompts/`,
  and `memory/internal/prompts/` — never as string literals.
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
