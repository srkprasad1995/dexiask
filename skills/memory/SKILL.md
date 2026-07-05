# Skill: memory

**Roles**: ask

You have a **memory** MCP server with long-term memory scoped as `user` (facts
about the person you're talking to), `repo` (facts about a specific repository),
and `global` (facts true across the whole workspace). Tools:

- `memory_view` — read a scope's index / an entry (`scope`, optional `scope_id`,
  optional `path`).
- `memory_search` — find existing memory by keyword before relying on it.
- `memory_write` — for you (a chat role) the only command is **`observe`**:
  record a durable observation to working memory. The dream/consolidation job
  later curates observations into lasting memory. You cannot create/edit
  consolidated entries directly — just observe.

## Use memory on every conversation

**At the start of a substantive turn**, if the question could benefit from prior
context, quickly `memory_view` the `user` scope (and the relevant `repo` scope if
the question is about a specific repo) so your answer reflects what you already
know about this person and codebase. Don't announce that you're checking memory —
just use what you find.

## Record what's worth remembering (`observe`)

**After a turn**, when you've learned something durable and reusable, record it
with `memory_write` command `observe`. One short, factual observation per call.
Choose the scope:

- `scope: "user"` — a stable fact/preference about the person (leave `scope_id`
  blank; it's filled in as their identity). E.g. "Prefers concise answers with
  file:line citations", "Works primarily on the payments service".
- `scope: "repo"` with `scope_id: "<repo id>"` — a durable fact about a repo's
  architecture, conventions, or gotchas you discovered. E.g. "auth is handled in
  internal/auth via GitHub OAuth", "the build uses make; entrypoint cmd/server".

Worth observing: stable preferences, recurring goals, non-obvious architecture or
conventions, decisions, and hard-won gotchas.

**Do NOT observe**: transient/one-off details, secrets or credentials, anything
you're unsure is durable, or a near-duplicate of something memory already holds
(search first if unsure). Prefer a few high-signal observations over many trivial
ones — silence is better than noise.
