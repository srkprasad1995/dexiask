# Contributing to Dexiask

Thanks for your interest in improving Dexiask! This guide covers how to get a
dev environment running, the conventions each service follows, and how to get a
change merged.

## Ground rules

- **Read [`CLAUDE.md`](./CLAUDE.md) first.** It documents the cross-service
  contracts (Agent Job Protocol, the SSE envelope, the shared `/workspace`
  mount) that a change must not break, and the coding guidelines each service
  follows.
- **Respect the non-goals.** Dexiask is deliberately single-user and
  Claude-only: no auth, teams, projects, runtime-config UI, memory, or
  multi-engine support. If your change reintroduces one of these, open an issue
  to discuss it first — it may belong in the upstream platform, not here.
- **Tests are mandatory.** Every change ships with tests, and the full suite
  must be green (`make test`) before you open a PR.

## Prerequisites

- **Docker + Docker Compose** — to run the full stack.
- **Go 1.23+** — `backend/`
- **Python 3.12+** — `engine/`, `indexer/`
- **Node 22 + pnpm 10** — `web/` (`corepack enable` gives you the pinned pnpm)
- API keys for a full run: an [Anthropic API key](https://console.anthropic.com/)
  and a [Voyage AI key](https://www.voyageai.com/). Unit tests do **not** need
  them.

## Running the stack

```bash
cp .env.example .env      # set ANTHROPIC_API_KEY, VOYAGE_API_KEY, DEXIASK_WORKSPACE_PATH
make up                   # build + start everything, web on http://localhost:25051
make logs                 # tail all services
make down                 # stop
make clean                # stop and wipe the DB + index
```

## Tests and linting

The top-level `Makefile` runs each suite in its own toolchain — the same
commands CI runs:

```bash
make test           # every suite: Go + engine + indexer + web
make test-backend   # cd backend && go test ./...
make test-engine    # cd engine  && pytest -q
make test-indexer   # cd indexer && pytest -q
make test-web       # cd web     && pnpm test
make lint           # go vet + ruff + eslint
make fmt            # gofmt + ruff format
```

### Per-service dev setup

Each service is independently developable — see its own `README.md` for detail.

- **backend/** (Go) — `go test ./...`, `go vet ./...`. Follows Clean
  Architecture (Handler → Service → Repository → GORM). Generate mocks with
  `go.uber.org/mock/mockgen`; never hand-write them.
- **engine/** and **indexer/** (Python) — create a venv, `pip install -e '.[dev]'`,
  then `pytest -q` and `ruff check .`. Code is typed and must be `ruff`-clean.
- **web/** (TypeScript) — `pnpm install`, then `pnpm test` (vitest),
  `pnpm lint` (eslint), and `pnpm typecheck` (tsc).

## Coding guidelines

These mirror `CLAUDE.md` — the source of truth:

- **Clean, modular, readable.** Match the surrounding file's style. Delete dead
  code in the same change; no commented-out blocks.
- **Surgical changes.** Touch only what the task needs; remove imports/vars you
  orphan.
- **No prompt text in code.** Agent prompts live in embedded `.md` files
  (`backend/internal/agent/prompts/`) and in `skills/*/SKILL.md`, never as
  string literals.
- **No mock/fake fallbacks in runtime code** — wire real infra and fail loudly
  when it's missing.

## Submitting a change

1. **Open an issue first** for anything non-trivial (a new feature, a behavior
   change, or anything touching a cross-service contract) so the approach can be
   agreed before you build it. Small fixes can go straight to a PR.
2. **Fork** the repo and branch from `main`.
3. Make the change, **add tests**, and run `make test` and `make lint` locally.
4. Keep commits focused with clear messages; explain the *why* in the PR
   description, and note any contract (`CLAUDE.md`) you touched.
5. **Open the PR** against `main`. CI runs the four suites above — get it green.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](./LICENSE).
