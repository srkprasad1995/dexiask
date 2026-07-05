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

Lean into memory: use it **proactively on most turns**. It's cheap, it makes you
better over time, and it's the main way this assistant gets smarter about this
person and codebase. Bias toward reading and recording a bit more, not less — but
never at the cost of noise (see the guardrails).

## Read at the start of most turns

Before answering anything non-trivial, quickly consult memory so your answer
reflects what you already know:

- `memory_view` the `user` scope (the person's preferences/goals), and
- `memory_view` the relevant `repo` scope when the question touches a specific
  repo, and/or `memory_search` for keywords from the question.

Do this by default — you don't need to be certain it'll help. Feel free to make
2–3 quick memory reads across scopes in a turn when they're plausibly relevant.
Don't announce that you're checking memory; just use what you find. Only skip it
for trivial one-liners (a greeting, a yes/no, a pure restate).

## Record generously each turn (`observe`)

**On most substantive turns, record one or more observations** — capture what a
future conversation would benefit from knowing. It's fine (encouraged) to write
**2–4 observations in a single turn** when you genuinely learned that much. One
short, factual observation per `memory_write` call. Choose the scope:

- `scope: "user"` — a stable fact/preference/goal about the person (leave
  `scope_id` blank; it's filled in as their identity). E.g. "Prefers concise
  answers with file:line citations", "Working on the payments refactor this week".
- `scope: "repo"` with `scope_id: "<repo id>"` — a durable fact about a repo's
  architecture, conventions, entrypoints, or gotchas you discovered. E.g. "auth
  is handled in internal/auth via GitHub OAuth", "the build uses make; entrypoint
  cmd/server".

Worth observing (generously): preferences, current focus and recurring goals,
non-obvious architecture, conventions, key entrypoints, decisions, and hard-won
gotchas — anything you'd want to have known at the start of this turn.

## Guardrails (the aggression has limits)

- **Never** record secrets, credentials, tokens, or PII.
- **De-dupe**: if you're unsure whether memory already holds something, briefly
  `memory_search` first; don't re-observe near-duplicates. It's fine to re-state
  a fact only when you've meaningfully refined or corrected it.
- Keep each observation short, specific, and durable — skip the truly transient
  (a one-off value, a temporary path). When a turn genuinely taught you nothing
  reusable, record nothing. Quality still beats volume; "a bit more" is the goal,
  not a flood.
