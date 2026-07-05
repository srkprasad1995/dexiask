# Dexiask Backend

The Go HTTP service at the center of Dexiask. It:

1. **Streams chat over SSE** by bridging to the Claude **engine** via the Agent Job Protocol.
2. **Persists** conversations, messages, and attachments in Postgres (GORM).
3. Handles **file-upload attachments** onto the shared `/workspace` volume (path-jailed).
4. **Authenticates users** (GitHub token login + optional OAuth), enforces `admin`/`member`
   roles + invites, and scopes every row to the resolved GitHub user id.
5. **Reverse-proxies** the indexer + memory control-planes (`/v1/indexer/*`, `/v1/memory/*`),
   forwarding each caller's identity so the indexer gates repos per user.
6. Runs a **Slack Socket Mode bot** as a second front-end onto the same chat service.

It is a **multi-user, single-workspace** backend: one mounted codebase (`WorkspaceID` is
fixed) while `UserID` is the real GitHub user id. One role for the agent (`ask`), one
runtime (`claude`). No teams/projects/config tables/analytics. See `internal/auth`.

## Architecture

Clean Architecture layers, wired with Uber Fx:

```
Handler → Service → Repository → GORM/Postgres
                 ↘ agent.Registry → ClaudeHTTPRuntime → engine POST /v1/jobs (NDJSON)
```

- `internal/agent` — the engine bridge. `Job`/`Event` (`protocol.go`), `HTTPRunner`
  (`runner_http.go`, NDJSON → `chan Event`), `RunManager` (`run_manager.go`, detached
  runs with a replayable event buffer), and the `ClaudeHTTPRuntime` adapter. Event type
  names are kept **byte-for-byte identical** to the engine output and the web SSE consumer.
- `internal/service/chat_service.go` — the orchestrator: resolve/create the conversation,
  persist the user turn + a running assistant placeholder, build the ask `Job` (system
  prompt from an embedded `.md`, read-only tools, `/skills`, the indexer MCP server,
  `sessionId` for resume), launch a detached run, and persist the assistant reply +
  returned `sessionId`. **Session continuity** works: the `sessionId` from the terminal
  `result` event is stored on `conversations.session_id` and sent back next turn.
- `internal/slack` — the Slack bot. Each Slack thread (`channel:thread_ts`) maps to one
  conversation (`slack_threads` table); the bot posts a placeholder and edits it via
  `chat.update` as `text.delta` events stream. It is inert unless both `SLACK_APP_TOKEN`
  and `SLACK_BOT_TOKEN` are set.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/chat/stream` | Start a turn; streams SSE (`data: <Event JSON>\n\n` … `data: [DONE]\n\n`) |
| GET  | `/v1/chat/stream?conversationId=…` | Resume an in-progress run (Last-Event-ID cursor) |
| POST | `/v1/chat/stop?conversationId=…` | Stop an in-progress run |
| GET  | `/v1/conversations` | List conversations |
| GET  | `/v1/conversations/{id}` | Message transcript |
| GET  | `/v1/conversations/{id}/messages` | Message transcript |
| POST | `/v1/attachments` | Upload a file (multipart) |
| GET  | `/v1/attachments/{id}` | Serve a file |
| ANY  | `/v1/indexer/*` | Reverse proxy to the indexer (prefix stripped) |
| GET  | `/healthz` | Liveness + DB ping |

## Configuration

See `.env.example`. Key variables: `DEXIASK_DB_DSN`, `DEXIASK_AGENT_URL`,
`DEXIASK_INDEXER_URL`, `DEXIASK_INDEXER_MCP_URL`, `DEXIASK_MODEL`, `DEXIASK_MAX_TOKENS`,
`DEXIASK_WORKSPACE_MOUNT`, `SLACK_APP_TOKEN`, `SLACK_BOT_TOKEN`. The container listens on
port **8080**.

## Development

```bash
go mod tidy
go build ./...
go test ./...          # unit tests (mocks via go.uber.org/mock/mockgen)
go vet ./...
go generate ./...      # regenerate mocks after changing a mocked interface
```

Test mocks are generated with `mockgen` into `test/mocks` (repository interfaces) and
`test/svcmocks` (service interfaces — split out to avoid an import cycle in the service
package's internal tests). Never hand-write mocks.

## Data model

`AutoMigrate` manages exactly four tables: `conversations` (id, session_id, title,
timestamps), `messages`, `attachments`, and `slack_threads` (the Slack thread → conversation
mapping).
