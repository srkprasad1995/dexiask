# Agent Job Protocol (Dexiask engine)

This document is the contract the engine implements. A Go orchestrator speaks it
to drive an ask-mode assistant turn, scoped to the read-only `ask` role.

## Transport

An always-on HTTP service.

- **Request**: `POST /v1/jobs` — JSON body (the Job object below).
- **Response**: streaming `application/x-ndjson` — one JSON event object per
  line, terminated by a `result` or `error` event.
- **Cancel**: `POST /v1/jobs/{run_id}/cancel` — aborts an in-progress job. The
  `run_id` is returned in the `X-Run-Id` response header.
- **Health**: `GET /healthz` — returns `{"status": "ok"}`.
- **Context cancel**: when the caller's HTTP request is cancelled, the engine
  aborts the current run.

### Tracing (observability)

The orchestrator may inject a W3C **`traceparent`** header on `POST /v1/jobs`.
The engine extracts it (FastAPI auto-instrumentation) so its `job` span becomes a
child of the caller's span. Transparent to the protocol body; the engine MAY
ignore it.

## Job (request body)

```json
{
  "role":          "ask",
  "model":         "claude-sonnet-5",
  "systemPrompt":  "You are a read-only assistant...",
  "allowedTools":  ["Read", "Glob", "Grep", "WebSearch", "WebFetch", "AskChoice"],
  "permissionMode":"dontAsk",
  "skillsPath":    "/skills",
  "workspacePath": "/workspace/<workspaceId>",
  "sessionId":     "sess-abc123",
  "sessionStorePath": "/workspace/<workspaceId>/conversations/<id>/session",
  "baseUrl":       "https://api.anthropic.com",
  "apiKey":        "sk-ant-...",
  "maxTokens":     8192,
  "maxTurns":      20,
  "effort":        "high",
  "thinking":      { "type": "adaptive", "display": "summarized" },
  "fallbackModel": "claude-sonnet-5",
  "betas":         ["context-1m-2025-08-07"],
  "mcpServers": [
    { "name": "indexer", "type": "http", "url": "https://mcp.example.com/", "description": "Codebase search", "headers": { "Authorization": "Bearer ..." }, "deferLoading": false }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Look at this screenshot and the config file.",
      "attachments": [
        { "kind": "image", "path": "/workspace/<workspaceId>/attachments/<conv>/<id>-shot.png", "mediaType": "image/png", "filename": "shot.png" },
        { "kind": "file",  "path": "/workspace/<workspaceId>/attachments/<conv>/<id>-config.yaml",  "mediaType": "text/yaml", "filename": "config.yaml" }
      ]
    },
    { "role": "assistant", "content": "..." },
    { "role": "user",      "content": "Explain the repository layer." }
  ]
}
```

| Field | Type | Description |
|---|---|---|
| `role` | string | Only `ask` is supported by this deployment. |
| `model` | string | Model ID. Empty → `CLAUDE_ENGINE_MODEL` default. |
| `systemPrompt` | string | System prompt injected by the orchestrator. |
| `allowedTools` | string[] | Read-only tool whitelist. `Read`/`Glob`/`Grep` are jailed to `workspacePath`; `WebSearch`/`WebFetch` are SDK built-ins; `AskChoice` asks structured questions. MCP tools arrive via `mcpServers[]`. |
| `permissionMode` | string | `dontAsk` (auto-approve pre-allowed read tools) or `default`. |
| `skillsPath` | string | Mounted path of skill packs (read-only). `*/SKILL.md` is appended to the system prompt. |
| `workspacePath` | string | Per-workspace working dir (the agent cwd, and the read-tool jail root). Created if missing. |
| `sessionId` | string? | Engine session ID; when set, the SDK resumes the prior session instead of replaying history. |
| `sessionStorePath` | string? | Per-conversation directory the SDK session store points at. Omitted → SDK default (`~/.claude`). |
| `baseUrl` | string? | `ANTHROPIC_BASE_URL` override. Empty → `ANTHROPIC_BASE_URL` env, else provider default. |
| `apiKey` | string? | `ANTHROPIC_API_KEY` override. Empty → the engine's `ANTHROPIC_API_KEY` env. Applied to the SDK via `options.env`. |
| `maxTokens` | int? | Output-token cap. Omitted → `Settings.max_tokens` (8192). |
| `maxTurns` | int? | Agent loop iterations. Omitted → `Settings.max_turns` (20). |
| `effort` | string? | `low`/`medium`/`high`/`xhigh`/`max`. Omitted → SDK default. |
| `thinking` | object? | `{ "type": "adaptive", "display": "summarized"\|"omitted" }`. Omitted → off. |
| `fallbackModel` | string? | Model the SDK falls back to if the primary is unavailable. |
| `betas` | string[]? | Claude Agent SDK beta flags. |
| `mcpServers` | MCPServer[]? | Remote MCP servers to attach. `deferLoading: false` attaches natively; `deferLoading: true` is exposed lazily via the in-process `mcp_router` (`list_mcp_servers`/`search_tools`/`call_tool`) so its tool defs stay out of context until searched. |
| `messages` | Message[] | Conversation history. Each has `role`, `content`, and optional `attachments[]`. When `sessionId` is set, the SDK replays prior context so only the latest user message is used. |

Each `Attachment`: `kind` (`image`/`file`), `path` (absolute container path),
`mediaType`, `filename`. Images ≤ 5 MB are sent as Anthropic base64 image blocks;
larger images and non-image files are delivered as a `/workspace` path reference
the `Read` tool can open.

## Events (response NDJSON)

Event names match the SSE envelope in `web/src/lib/chat/go-sse-to-ui.ts`
so the orchestrator is a thin re-framer, not a translator.

### Text streaming
```json
{ "type": "text.start", "id": "b1" }
{ "type": "text.delta", "id": "b1", "text": "The repository layer..." }
{ "type": "text.stop",  "id": "b1" }
```

### Extended thinking / reasoning
```json
{ "type": "thinking.start", "id": "r1" }
{ "type": "thinking.delta", "id": "r1", "text": "Let me consider..." }
{ "type": "thinking.stop",  "id": "r1" }
```

### Tool use
```json
{ "type": "tool.start",       "id": "t1", "name": "Read" }
{ "type": "tool.input_delta", "id": "t1", "json": "{\"path\":" }
{ "type": "tool.input_done",  "id": "t1", "name": "Read", "input": { "path": "..." } }
{ "type": "tool.result",      "id": "t1", "result": "..." }
```

#### Interactive questions (`AskChoice`)

The engine exposes an `AskChoice` tool so the model can ask structured
multiple-choice questions instead of asking in prose. No new event type — the
question payload rides in the tool *input* (`tool.start` / `tool.input_done`),
which the UI renders as clickable options. This is **ask-then-resume**: the tool
returns synchronously so the turn ends cleanly with a `result` + `sessionId`; the
user's selection arrives as the next user turn and the engine resumes the session.

> Named `AskChoice`, not `AskUserQuestion` — the latter is a built-in Claude Code
> tool needing an interactive permission prompt, which is denied in the engine's
> headless `dontAsk` mode (the engine adds `AskUserQuestion` to `disallowed_tools`).

### Agent step (visible progress card in the UI)
```json
{ "type": "agent.step", "step": 1, "tool": "Read" }
```

### Terminal events
```json
{ "type": "result", "status": "success", "model": "claude-sonnet-5", "sessionId": "sess-abc123",
  "usage": { "input_tokens": 1240, "output_tokens": 380, "cache_read_tokens": 9100, "cache_creation_tokens": 0 },
  "costUsd": 0.0123, "numTurns": 4 }
{ "type": "error",  "message": "No API credentials available." }
```

`result` signals successful completion; `error` a fatal failure. Exactly one of
the two is the last event before the body closes. The `sessionId` in `result` is
the Claude Agent SDK session id the orchestrator persists and sends back to resume.

**Usage / cost.** The `result` event carries additive fields: normalized token
`usage` (all four keys, 0 where unreported), the engine-reported `costUsd` when
the SDK provides one, and `numTurns`. All optional; absent ⇒ unknown.
```
