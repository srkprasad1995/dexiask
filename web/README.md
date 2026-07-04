# dexiask-web

The **web** frontend for [Dexiask](../) — a Next.js (App Router) **BFF** and
chat UI. The browser talks only to this app's route handlers under
`src/app/api/**`, which proxy the Dexiask Go backend and translate its SSE chat
stream into the AI SDK UI-message stream.

```
Browser ──▶ Next.js BFF (app/api/**) ──▶ Dexiask Go backend (:8080)
  useChat ◀── AI SDK UI message stream ◀── go-sse-to-ui.ts ◀── SSE ── engine ── model
```

Single-tenant, no auth. Two surfaces:

- **Chat** (`/`) — stream a conversation with the Claude agent over your
  mounted codebase, with rich rendering (Markdown, Shiki dual-theme code, KaTeX
  math, Mermaid diagrams, sandboxed HTML previews) and file attachments.
- **Indexer** (`/indexer`) — register repositories (git URL or a path under
  `/workspace`), (re)index them, watch status, and run semantic code search.

## Stack

Next.js 16 · React 19 · TypeScript · Tailwind v4 · shadcn/ui (on `@base-ui/react`)
· AI SDK v6 (`ai` + `@ai-sdk/react`) · Streamdown (+ code/mermaid/math plugins) ·
Shiki · KaTeX · TanStack Query · next-themes · Vitest + Testing Library.

## Develop

```bash
pnpm install
pnpm dev            # http://localhost:3000
pnpm test           # vitest
pnpm build          # production build (standalone output)
pnpm lint           # eslint
pnpm typecheck      # tsc --noEmit
```

Node 20+, **pnpm**. The dev server and container both serve on port **3000**.

## Configuration

| Variable | Purpose | Default |
|---|---|---|
| `GO_BACKEND_URL` | Base URL of the Dexiask Go backend the BFF proxies to. | `http://localhost:8080` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Optional OTLP endpoint for server-side traces. No-op when unset. | — |
| `OTEL_SERVICE_NAME` | Service name reported in traces. | `dexiask-web` |

Copy `.env.example` to `.env.local` for local dev. In `docker-compose.yml`
(repo root) `GO_BACKEND_URL` points at the `backend` service.

## Key files

- `src/lib/chat/go-sse-to-ui.ts` — **the linchpin**: the single point that
  translates the Go SSE envelope into AI SDK UI-message-stream chunks. Its event
  `type` names must match what the Go backend emits.
- `src/components/chat/chat.tsx` — the `useChat` host (transport, send/stop).
- `src/components/chat/message.tsx` — renders a message by its `parts[]`.
- `src/app/api/chat/route.ts` — chat relay (POST start + GET resume).
- `src/app/api/indexer/[[...path]]/route.ts` — indexer proxy → `/v1/indexer/*`.
- `src/app/globals.css` — design tokens (the "Precision Instrument" language).

## Docker

```bash
docker build -t dexiask-web .
docker run -p 3000:3000 -e GO_BACKEND_URL=http://host.docker.internal:8080 dexiask-web
```

Multi-stage build using Next.js `standalone` output; runs `node server.js` as a
non-root user. Health probe: `GET /api/healthz`.
