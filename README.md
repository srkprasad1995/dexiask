<div align="center">

# dexiask

**Chat with a Claude agent over your codebase — with semantic code search.**

An open-source AI **Ask** platform for your codebase.
Sign in with GitHub, chat with a Claude engine over one mounted codebase, backed by a
Qdrant indexer and per-user long-term memory.
Bring your own API keys, `docker compose up`, and go.

🌐 **[dexiask.com](https://dexiask.com)**

</div>

---

## What it is

Dexiask is a small, self-hostable stack for **asking questions about a codebase**
and getting rich, streamed answers from a Claude agent that can read your files,
search the web, and run **semantic code search** over an indexed repository.

It stays deliberately focused — one workspace (your mounted codebase), no teams, no
projects, no config UI — while supporting multiple users:

- Claude agent (ask mode, read-only)
- **Sign in with a GitHub token** (OAuth optional) — per-user conversation history (in
  the sidebar, paginated), memory, and MCP servers, with **admin/member roles** and
  admin-only invites (or run with no auth configured for a single local dev user)
- **Per-user repo access** — each member only sees the repos their own GitHub token
  grants, validated inside the indexer
- Streaming chat with rich rendering (code, mermaid, math)
- File upload (attach files to a message)
- Custom skills (drop-in `SKILL.md` packs)
- Semantic code indexer (Qdrant + Voyage), default branch, plus optional **LLM-generated
  domain docs** searchable alongside code
- **Long-term memory** — the agent records observations and a "dream" judge consolidates
  them into durable user/repo/global memory injected into later answers
- Two-way Slack bot (Socket Mode)

## Why Dexiask

**Index once, ask anything.** Point it at a repo, index the default branch a single
time, and from then on ask questions in plain language — "where is auth handled?",
"what calls this function?", "how does a request flow end to end?". No wiki, no
docs, no comments required: the code *is* the source of truth, and the agent reads
it directly via semantic + lexical search.

- **No documentation debt.** Docs rot the moment they're written; the index tracks
  the actual committed code, so answers reflect what's really there — not a stale
  README from six months ago.
- **Decentralized knowledge.** New joiners, on-call engineers, PMs, or anyone
  outside the original authors can get accurate answers without interrupting the
  one person who "knows that part." The codebase stops being tribal knowledge
  locked in a few heads.
- **Speed.** Seconds to an answer instead of an afternoon spent grepping, reading,
  or waiting on a Slack reply. Ask from the browser, or `@mention` the bot right in
  Slack where the question came up.
- **Onboarding & code archaeology.** Ramp on an unfamiliar service, understand a
  legacy module, or trace "why is this here?" through `git blame`/`git log` — all
  from a conversation.
- **Yours, self-hosted.** Runs on your machine with your keys; your code never
  leaves your infrastructure beyond the model/embedding API calls you opt into.

## Architecture

```
                            ┌─────────────┐
 Browser ──HTTP/SSE──▶ web  │  Next.js    │
                       BFF  │  (login +   │
                            │   chat + …) │
                            └──────┬──────┘
                                   │ SSE
                            ┌──────▼──────┐        ┌───────────┐
 Slack ──Socket Mode──────▶ backend      │──MCP──▶│  indexer  │──▶ Qdrant
                            │  (Go)       │        │ (Python)  │──▶ git mirrors (FS)
                            │  chat · SSE │        └───────────┘
                            │  auth       │──MCP──▶┌───────────┐
                            │  attachments│        │  memory   │──▶ FS volume
                            │  Postgres   │        │   (Go)    │
                            └──────┬──────┘        └───────────┘
                                   │ Agent Job Protocol
                                   ▼
                            ┌─────────────┐
                            │  engine     │
                            │ (Python,    │
                            │  Claude SDK)│
                            └─────────────┘
```

- **web** — Next.js BFF + login + chat/indexer/memory UI. The only thing the browser talks to.
- **backend** — Go. Streams chat as SSE, bridges to the engine over the **Agent Job
  Protocol**, handles **GitHub OAuth** + sessions, stores conversations/messages/
  attachments in Postgres, proxies the indexer + memory, and runs the Slack bot.
- **engine** — Python. Runs the **Claude Agent SDK**; reads `ANTHROPIC_API_KEY` from
  env; loads skill packs from `/skills`.
- **indexer** — Python. Semantic + lexical code search (and optional generated domain
  docs) over the mounted codebase, exposed to the engine as an MCP server. Qdrant for
  vectors, the filesystem for git mirrors.
- **memory** — Go. FS-backed user/repo/global memory exposed as an MCP server, with a
  periodic "dream" consolidation judge. No database.
- **qdrant / postgres** — vector store and relational store.

One **shared `/workspace` mount** is the codebase the agent reads and the indexer
indexes (read-only for the indexer).

## Quickstart

**Prerequisites:** Docker + Docker Compose, an [Anthropic API key](https://console.anthropic.com/),
and a [Voyage AI key](https://www.voyageai.com/) (for code embeddings).

```bash
git clone <your-fork> dexiask && cd dexiask
cp .env.example .env
#  → edit .env: set ANTHROPIC_API_KEY and VOYAGE_API_KEY,
#    and point DEXIASK_WORKSPACE_PATH at the repo you want to explore.
make up          # or: docker compose up --build -d
```

Open **http://localhost:25051** and start chatting. `make logs` to tail, `make down`
to stop, `make clean` to wipe the DB + index.

## Indexing a repository

The indexer indexes the **default branch** of any git repo under your mounted
`/workspace`.

1. Set `DEXIASK_WORKSPACE_PATH` in `.env` to a directory containing the repo.
2. Open the **Indexer** page in the UI → **Add repo** (git URL or a path under
   `/workspace`) → **Reindex**.
3. Ask the agent a question about the code — it will call `semantic_search`
   automatically (you'll see the tool card in the chat).

### Private repos (central token) & per-user gating

Indexing uses **one central git token** an **admin** sets on the **Indexer** page →
**Git access token** → paste a GitHub PAT → **Save**. It is held server-side by the
indexer, persisted `0600`, and **never returned to the browser** — injected as an
`Authorization` header via `GIT_CONFIG_*`, never placed in a mirror's stored remote
URL. Clear it any time with **Clear**.

Members still only see the repos **their own GitHub token** can access: the indexer
validates every member's token against each repo via the GitHub API *itself* (cached),
so a forged header — or a direct hit on the indexer port — only ever reveals what the
caller's token grants. Admins are unrestricted; local / non-GitHub repos are admin-only.
This gating (across `semantic_search`, repo-scoped tools, and repo listings) turns on
when `DEXIASK_INTERNAL_TOKEN` is set on both the backend and the indexer.

## Sign in & roles

Dexiask is multi-user. The **primary login is a GitHub token**: paste a GitHub PAT on
`/login` — it's validated against the GitHub API, stored AES-GCM encrypted, and becomes
your identity (also used for per-user repo access). Auth turns on once the session infra
is configured — set `DEXIASK_SESSION_SECRET`, `DEXIASK_TOKEN_ENC_KEY`
(`openssl rand -hex 32`), and web `AUTH_ENABLED=true`. Leave them blank to run in
**dev-fallback** mode (a single local **admin** user, no login) — the zero-config default.

**GitHub OAuth** is optional on top: create a GitHub OAuth app (callback
`http://localhost:25051/api/auth/callback`) and set `DEXIASK_GITHUB_CLIENT_ID` /
`_SECRET` + `DEXIASK_OAUTH_CALLBACK_URL` to add a one-click sign-in button.

**Two roles — `admin` and `member`.** The first user to sign in bootstraps as admin;
everyone else needs an admin-created **invite** (keyed by GitHub login, consumed on first
login) or login is refused. Members get chat, and their own history / memory / MCP
servers. Admin-only: the **Team** page (invites + roster), MCP-server management, indexer
mutations (reindex / register / the central git token), and memory edits + consolidation.
Conversations, MCP servers, and memory are all scoped per user.

## Memory

The agent records observations about you and your repos during chat. A periodic
**dream** run (an LLM "judge") consolidates them into durable **user / repo / global**
memory, which is injected into later answers so the assistant improves over time. Browse
what it remembers — and trigger a consolidation manually — on the **Memory** page.
Backed by the `memory` service on the `memory-data` volume (no database); tune with
`DEXIASK_DREAM_MODEL` / `DEXIASK_DREAM_INTERVAL` (`0` disables the loop).

## Custom MCP servers

Beyond the built-in code indexer, you can connect your own **MCP servers** so
the agent gains their tools. Open the **MCP** page → add a server (name,
transport `http` or `sse`, URL, and optional request headers for auth) and
toggle it **Enabled**. Every enabled server is injected into each of **your** chat
turns alongside the indexer + memory. Servers are scoped per user; header values
(which may hold auth secrets) are stored in Postgres in plaintext.

## Custom skills

Drop a folder into `skills/` containing a `SKILL.md` and restart the engine
(`make restart`). The engine appends every `skills/*/SKILL.md` to the agent's system
prompt. Bundled packs: `code-explain`, `helpful`, `voice`. See `skills/README.md`.

## Slack bot (optional)

Dexiask can run a two-way Slack bot over **Socket Mode** (no public URL needed).

1. Create a Slack app → enable **Socket Mode** → generate an **App-Level Token**
   (`xapp-…`) with scope `connections:write`.
2. Under **OAuth & Permissions**, add bot scopes: `app_mentions:read`, `chat:write`,
   `im:history`, `im:read`, `im:write`. Install the app and copy the **Bot Token**
   (`xoxb-…`).
3. Under **Event Subscriptions**, subscribe to bot events: `app_mention`,
   `message.im`.
4. Put both tokens in `.env` (`SLACK_APP_TOKEN`, `SLACK_BOT_TOKEN`) and
   `make restart`.
5. DM the bot or `@mention` it in a channel — its reply streams into the thread.

Leave the tokens blank to keep the bot disabled.

## Configuration

Everything is env-driven — see `.env.example` for the full list. The essentials:

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | Claude engine credential (required) |
| `VOYAGE_API_KEY` | Indexer embedding credential (required) |
| `DEXIASK_MODEL` | Claude model for ask mode (default `claude-sonnet-5`) |
| `DEXIASK_WORKSPACE_PATH` | Host codebase mounted at `/workspace` |
| `DEXIASK_SESSION_SECRET` / `_TOKEN_ENC_KEY` | Session signing + GitHub-token encryption — enables auth (token login) |
| `DEXIASK_GITHUB_CLIENT_ID` / `_SECRET` | Add optional GitHub **OAuth** on top of token login (blank = token login only) |
| `DEXIASK_INTERNAL_TOKEN` | Shared backend↔indexer secret; enables per-user repo gating in the indexer |
| `DEXIASK_ENABLE_DOMAIN_DOCS` | Generate + index LLM domain docs during indexing |
| `DEXIASK_DREAM_MODEL` / `_DREAM_INTERVAL` | Memory consolidation model + cadence (`0` disables) |
| `SLACK_APP_TOKEN` / `SLACK_BOT_TOKEN` | Enable the Slack bot |

## Development

Each service is independently developable — see the per-service `README.md`
(`backend/`, `engine/`, `indexer/`, `memory/`, `web/`).

```bash
make test          # run every suite: Go (backend + memory) + engine + indexer + web
make test-backend  # Go unit tests
make test-engine   # pytest (engine)
make test-indexer  # pytest (indexer)
make test-web      # vitest
make lint          # vet + ruff + eslint
```

## License

[MIT](./LICENSE).
