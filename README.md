<div align="center">

# dexiask

**Chat with a Claude agent over your codebase — with semantic code search.**

An open-source AI **Ask** platform for your codebase.
One user, one Claude engine, one mounted codebase, one Qdrant-backed indexer.
Bring your own API keys, `docker compose up`, and go.

🌐 **[dexiask.com](https://dexiask.com)**

</div>

---

## What it is

Dexiask is a small, self-hostable stack for **asking questions about a codebase**
and getting rich, streamed answers from a Claude agent that can read your files,
search the web, and run **semantic code search** over an indexed repository.

It is deliberately minimal — no accounts, no teams, no projects, no config UI:

- Claude agent (ask mode, read-only)
- Streaming chat with rich rendering (code, mermaid, math)
- File upload (attach files to a message)
- Custom skills (drop-in `SKILL.md` packs)
- Semantic code indexer (Qdrant + Voyage), default branch
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
                       BFF  │  (chat +    │
                            │   indexer)  │
                            └──────┬──────┘
                                   │ SSE
                            ┌──────▼──────┐        ┌───────────┐
 Slack ──Socket Mode──────▶ backend      │──MCP──▶│  indexer  │──▶ Qdrant
                            │  (Go)       │        │ (Python)  │──▶ git mirrors (FS)
                            │  chat · SSE │        └───────────┘
                            │  attachments│──Agent Job Protocol──┐
                            │  Postgres   │                      ▼
                            └─────────────┘               ┌─────────────┐
                                                          │  engine     │
                                                          │ (Python,    │
                                                          │  Claude SDK)│
                                                          └─────────────┘
```

- **web** — Next.js BFF + chat UI + indexer page. The only thing the browser talks to.
- **backend** — Go. Streams chat as SSE, bridges to the engine over the **Agent Job
  Protocol**, stores conversations/messages/attachments in Postgres, proxies the
  indexer, and runs the Slack bot.
- **engine** — Python. Runs the **Claude Agent SDK**; reads `ANTHROPIC_API_KEY` from
  env; loads skill packs from `/skills`.
- **indexer** — Python. Semantic + lexical code search over the mounted codebase,
  exposed to the engine as an MCP server. Qdrant for vectors, the filesystem for git
  mirrors.
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

### Private repos (git token)

To index a **private** repository, set a git access token on the **Indexer**
page → **Git access token** → paste a token (e.g. a GitHub PAT) → **Save**. The
indexer uses it to clone/fetch over HTTPS. The token is held server-side by the
indexer, persisted to its data volume with `0600` permissions, and **never
returned to the browser** (the UI only shows whether one is configured). It is
injected as an `Authorization` header via `GIT_CONFIG_*`, never placed in a
mirror's stored remote URL. Clear it any time with **Clear**. This is a
single-user local tool, so plaintext-at-rest is acceptable.

## Custom MCP servers

Beyond the built-in code indexer, you can connect your own **MCP servers** so
the agent gains their tools. Open the **MCP** page → add a server (name,
transport `http` or `sse`, URL, and optional request headers for auth) and
toggle it **Enabled**. Every enabled server is injected into each chat turn
alongside the indexer. Header values (which may hold auth secrets) are stored in
Postgres in plaintext — acceptable for this single-user local tool.

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
| `SLACK_APP_TOKEN` / `SLACK_BOT_TOKEN` | Enable the Slack bot |

## Development

Each service is independently developable — see the per-service `README.md`
(`backend/`, `engine/`, `indexer/`, `web/`).

```bash
make test          # run every suite: Go + engine + indexer + web
make test-backend  # Go unit tests
make test-engine   # pytest (engine)
make test-indexer  # pytest (indexer)
make test-web      # vitest
make lint          # vet + ruff + eslint
```

## License

[MIT](./LICENSE).
