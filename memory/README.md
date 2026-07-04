# memory

The FS-backed memory service for Dexiask, exposed as an **MCP server** and a small
REST API. It owns long-term memory at three scopes — **global**, **user** (per GitHub
user), and **repo** (per indexed repo) — and runs the **dream** consolidation judge that
curates raw observations into durable memory. No database; all state lives on the
`memory-data` volume under `<root>/<workspace>/memory/`.

## Surface

- `POST /mcp` — streamable-HTTP MCP. Tools:
  - `memory_view` — read a scope's index / topic / entry.
  - `memory_search` — find entries by content.
  - `memory_write` — `observe` (chat roles) and `create`/`update`/`delete`/`clear_working`
    (dream role only), plus `list_scopes`.
- `GET /v1/memory/{scopes,entries,topics,log,working,digest}`, entry CRUD, `POST
  /v1/memory/{purge,archive-stale}` — the FE memory browser + the backend's digest fetch.
- `POST /v1/consolidate` — run one dream consolidation now.
- `GET /healthz`.

Every request must carry `X-Workspace-Id` (plus `X-User-Id`, `X-Role`, and optional
`X-Writable-Scopes`). Unscoped requests are refused. Chat roles may only record
observations; the dream role curates consolidated memory.

## How the dream judge works

The scheduler fires every `DEXIASK_DREAM_INTERVAL` (an FS flock guards each tick). It
builds a context from each scope's *unprocessed* working observations and submits a job to
the Dexiask engine (`role: "ask"`, with the dream distinction carried by `X-Role: dream` on
the memory MCP + the `dream.md` prompt). The engine's LLM promotes/merges/discards, writing
consolidated entries back through this service's MCP, then `clear_working` marks the
observations processed so future dreams only see newer ones.

## Config

| Variable | Purpose |
|---|---|
| `DEXIASK_MEMORY_ROOT` | Memory volume mount (default `/memory`). |
| `DEXIASK_MEMORY_MCP_SELF_URL` | Engine-reachable `/mcp` URL, stamped into dream jobs. |
| `DEXIASK_AGENT_URL` | Engine `/v1/jobs` base (dream jobs). |
| `DEXIASK_DREAM_MODEL` | Model for the consolidation run (defaults to `DEXIASK_MODEL`). |
| `DEXIASK_DREAM_INTERVAL` | How often the dream runs (`0` disables the loop). |

## Development

```bash
make build   # go build ./...
make test    # go test ./...
make vet     # go vet ./...
make run     # run locally (memory root defaults to /memory)
```
