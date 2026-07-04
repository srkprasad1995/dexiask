# dexiask-indexer

Semantic + lexical **code search** over a mounted codebase, exposed to the agent
engine as an **MCP server over streamable HTTP** (`/mcp`) and to the Go backend
as a small **REST control plane**. Part of [Dexiask](../README.md).

It is a single-tenant, default-branch-only, database-free service: it keeps a
bare git mirror per repo, embeds each repo's primary branch into
[Qdrant](https://qdrant.tech) with [Voyage](https://voyageai.com) code
embeddings, and answers search / git-introspection / high-level-doc queries as
token-budgeted results.

## What it does

- **Semantic search** — code embedded into Qdrant (Voyage `voyage-code-3` by
  default, dim 1024), ranked, returned as token-budgeted pointers.
- **Lexical search** — `git grep` / glob / read against the primary branch
  (always fresh, no separate index).
- **Git introspection** — read-only `log` / `blame` / `diff` / `show` /
  `branches` / `file-at-rev`.
- **High-level docs** — a maintained structural skeleton (dirs, entrypoints,
  README, ranked symbols) + optional LLM overview for fast repo understanding.
- **TOON or JSON** output per request, with per-call token budgets.

## Design

The indexer keeps a **bare, single-branch mirror** per repo and indexes the
**primary-branch tip**. Indexing is a **convergent reconcile**: the index is made
a pure function of the tip — changed paths are re-embedded, removed paths are
deleted. Point ids are deterministic, so re-running (including on startup) is
**idempotent**. State (the `path -> blob_sha` map + last commit) and the per-repo
lock are in-memory; there is no database. Git mirrors, doc caches, and the
persisted repo registry live on the shared filesystem under `DEXIASK_DATA_DIR`.

The service **degrades gracefully**: if Qdrant or the Voyage key is unavailable
it still starts and serves the lexical / git / read / docs tools off the mirror;
only `semantic_search` is disabled.

## Configuration

All settings use the `DEXIASK_` env prefix (see `.env.example`). Key vars:

| Var | Default | Purpose |
|---|---|---|
| `DEXIASK_VOYAGE_API_KEY` | — | Voyage embedding key (env only, never persisted) |
| `DEXIASK_QDRANT_URL` | `http://localhost:6333` | Qdrant endpoint |
| `DEXIASK_EMBEDDING_MODEL` | `voyage-code-3` | Embedding model |
| `DEXIASK_EMBEDDING_DIM` | `1024` | Vector dimension |
| `DEXIASK_WORKSPACE_ROOT` | `/workspace` | Where local repos are mounted |
| `DEXIASK_DATA_DIR` | `/var/lib/dexiask-indexer` | Mirrors, doc cache, `repos.json` |

## Quickstart

```bash
python3.12 -m venv .venv && . .venv/bin/activate
pip install -e '.[dev,chunking]'
pytest -q                       # run the suite (no external services needed)

cp .env.example .env            # set DEXIASK_VOYAGE_API_KEY
python -m indexer               # serves on :8080 (needs Qdrant for semantic search)
curl localhost:8080/healthz     # {"status":"ok"}
```

Register a repo (by URL or by a path under `/workspace`), then index it:

```bash
curl -XPOST localhost:8080/v1/repos -d '{"id":"flask","url":"https://github.com/pallets/flask.git"}'
curl -XPOST localhost:8080/reindex -d '{"repo":"flask"}'
curl localhost:8080/v1/status
```

## HTTP surface

REST control plane:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/healthz` | `{"status":"ok"}` |
| `GET` | `/v1/status` | Per-repo indexed state |
| `GET` | `/v1/repos` | Registered repos with clone URLs |
| `POST` | `/v1/repos` | Register a repo (git URL or `/workspace` path) |
| `DELETE` | `/v1/repos/{id}` | Deregister a repo |
| `POST` | `/reindex` | Reindex one repo (`{"repo":...}`) or all |
| `POST` | `/v1/index/{repo}` | Reindex one repo |
| `GET` | `/v1/tools` | Tool catalog (mirrors the MCP tools) |
| `POST` | `/v1/tools/{name}` | Invoke a tool over REST |

MCP (`POST /mcp`, streamable HTTP) exposes: `semantic_search`, `get_chunk`,
`lexical_search`, `glob`, `read_range`, `explore`, `git_log`, `git_branches`,
`git_blame`, `git_diff`, `git_show`, `git_file_at_rev`, `get_overview`,
`get_docs`, `list_repos`.

## Layout

| Path | Role |
|---|---|
| `indexer/config/` | repo registry (yaml + API) |
| `indexer/git/` | bare-mirror management + read-only plumbing |
| `indexer/pipeline/` | primary-branch reconcile + in-memory state |
| `indexer/embedding/` | pluggable embedding providers (voyage / openai / hash) |
| `indexer/store/` | Qdrant collection + payload schema |
| `indexer/mcp/` | streamable-HTTP MCP server, tools, TOON/JSON formatting |
| `indexer/docs/` | structural skeleton + optional LLM overview |
| `indexer/scheduler/` | fetch + reconcile loop |
