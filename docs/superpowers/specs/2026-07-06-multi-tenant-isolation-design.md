# Dexiask Multi-Tenant Isolation Model — Design

**Date:** 2026-07-06
**Status:** Approved design (isolation model only; see *Deferred* for out-of-scope subsystems)
**Scope:** The tenancy & isolation foundation for hosting Dexiask as a central multi-tenant
cloud, while keeping self-hosting a first-class single-tenant mode on the same codebase.

---

## 1. Problem & goals

Dexiask today is **multi-user but single-workspace**: every product row is scoped to a real
GitHub `UserID`, but `WorkspaceID` is pinned to the constant `"dexiask"` and all state lives
in one shared set of volumes (one `/workspace` mount, one Qdrant, one `memory-data` volume,
one Postgres). Users are separated only *logically* — a `UserID` column, a `user/<id>` memory
subdirectory, a repo filter — inside physically shared storage. That is correct for a
self-hosted deployment where everyone is trusted, and unsafe for a public cloud where mutually
untrusting tenants share infrastructure.

This design introduces a **tenancy & isolation model** so Dexiask can run centrally hosted and
multi-tenant, with data-privacy and cross-tenant isolation strong enough to make defensible
privacy claims — **without forking the codebase**: the same binaries run self-hosted
single-tenant with zero config.

**Goals**
- A crisp, physical tenant boundary that mutually untrusting tenants cannot cross.
- Preserve today's exact self-hosted behavior as one setting of a single flag.
- Keep compute **shared and dense** — cost must scale with active load, not tenant count.
- A provable data-deletion story (crypto-shredding).
- One code path for solo, team, and enterprise tenants — no forked tenancy logic.

**Non-goals (this spec):** onboarding/workspace-lifecycle UI, billing/metering, audit logging,
the egress-proxy build, and RBAC beyond today's admin/member. Each is its own later spec →
plan cycle (see *Deferred*).

---

## 2. Governing principle: one deployment-mode flag

A single config value selects **every** behavior in this document:

```
DEXIASK_TENANCY_MODE = single | multi
```

- **`single` (default)** — today's exact zero-config behavior. One fixed org/workspace, one
  shared mount, one service key, dev-fallback admin, web tools enabled. Self-hosted.
- **`multi`** — everything in §3–§8. Central cloud.

There is one codebase and one test suite; both modes are exercised. No overlay repo, no fork.
When a behavior below differs by mode, it is called out explicitly.

---

## 3. Tenancy hierarchy

```
Org  (the tenant — physical isolation boundary + billing owner)
 └── Workspace  (logical sub-scope: a named project = 1+ repos)
      └── Members  (GitHub-authenticated users, roles; attribution within the org)
```

### Org — the physical boundary
The **org is the only hard isolation boundary.** Cross-org access is *never* permitted. The
org owns: its storage volume, its Qdrant partition, its memory tree, its data-encryption key,
and the compute mount used when its agents run (§4–§7). Billing and the enterprise
compute-dedication dial also sit at the org level.

The org is **always the physical unit, even for a solo user.** A solo self-serve signup
auto-provisions a **personal org (org-of-one)**; the UI never surfaces the word "org" for them
— they see only *themselves* and *their workspace(s)*. This yields exactly one tenancy code
path and a free upgrade path (solo → team → enterprise adds members / flips the compute dial;
no data migration).

| Shape | What the user sees | Under the hood | Compute |
|---|---|---|---|
| Solo self-serve | "Me + my workspace(s)" | Implicit personal org (1 member) | Shared pool |
| Team | Org explicit: members, invites, roles | Same org, >1 member | Shared pool |
| Enterprise | Org + isolation guarantees | Same org + dedicated dial | Dedicated pool/node |

The tiers differ **only** in what the UI surfaces and whether compute is dedicated — never in
the physical isolation mechanism.

### Workspace — a logical sub-scope
A workspace is a **named project that connects one or more repos** (e.g. a service + its infra
+ shared libs), with its own default index/memory/chat scope. It generalizes today's
"mount = a directory holding one or more git mirrors." A workspace is a **filter, not a wall**:
because everything inside an org is the same tenant's data, an org may run **cross-workspace
search** across its own workspaces. Workspace-scoped is the default; org-wide is an opt-in the
org controls (§5).

`WorkspaceID` is de-pinned from the `"dexiask"` constant and becomes a real per-org value.

| Dimension | Boundary type | Crossable? |
|---|---|---|
| **Org** | Physical (volume, Qdrant partition, memory tree, DEK, compute mount) | **Never** |
| **Workspace** | Logical (a filter within the org) | **Yes — the org's choice** |
| **User / Member** | Membership + attribution within the org | Per org policy |

---

## 4. Compute isolation — shared pool, per-turn org-scoped mount

**Key fact that sets the bar:** the agent is already **read-only**. Its entire toolset is
`Read, Glob, Grep, WebSearch, WebFetch, AskChoice` plus MCP (`semantic_search`, `memory_*`) at
`permissionMode: dontAsk` (`backend/internal/agent/protocol.go:129`). There is **no `Bash`, no
`Write`, no `Edit`** — the engine documents that write-oriented SDK modes are never sent. So
Dexiask is **not** hosting untrusted arbitrary code execution, and does **not** need
micro-VM/Firecracker-grade sandboxing. The only filesystem *write* anywhere is the backend
saving attachments under `.dexiask/` (path-jailed) — never the agent.

Two cross-tenant risks survive from read-only tools:
1. **Cross-tenant reads** — `Read`/`Glob`/`Grep` can read any file the worker process can see.
2. **Exfiltration via web egress** — `WebFetch`/`WebSearch` (handled in §6).

### The mechanism
- **A single, shared, bounded pool of stateless engine workers.** Not per-workspace, not
  per-org. The pool is sized to **concurrent active turns**, so idle tenants cost **zero**
  compute and 1,000 workspaces with ~10 asks in flight need ~10 workers. This satisfies the
  "share compute as much as possible" requirement.
- **Per-turn isolation invariant:** *a worker processes one org's turn at a time, and for that
  turn's duration has only that org's data in view* (a scoped bind-mount / mount-namespace
  exposing just that org's subtree), released when the turn ends. Concurrency comes from **more
  workers in the pool, never from multiplexing two orgs inside one process.** A single missed
  filter or path-traversal cannot leak, because the other org's files are not in the process's
  view at all.
- **Within an org, cross-workspace is allowed.** The scoped mount exposes the org's subtree;
  the *default* working set is the active workspace, but org-wide access is safe by
  construction (same tenant) and gated by §5's filter, not by the mount.

### Warm/sticky state must be externalized
Because compute is shared and any worker may pick up any turn, per-org warm state cannot live
inside a long-lived pod. It lives on **per-org persistent storage**, scope-mounted into the
worker that takes the turn:

| State | Where it lives under `multi` |
|---|---|
| Semantic index | Shared indexer/Qdrant, `org_id`-partitioned (§5) — already external ✅ |
| Memory | Shared memory service, per-org tree (§5) — already external ✅ |
| **Code mirror + Claude SDK session transcripts + attachments** | **Per-org persistent volume**, scope-mounted per turn (**new**) |

This is the deliberate trade the cost constraint demands: **per-org persistent *storage*
(cheap, idle-free) instead of per-workspace persistent *compute* (expensive when idle).** The
Claude SDK's native resume reads session-transcript files from disk, so those files must ride
on the per-org volume rather than a pod-local disk.

### The dial (not a separate design)
Worker lifecycle is a single knob, not a fork:
- **Starter / default:** shared warm pool with an **idle-reaper** (scale-to-zero after N
  minutes). Idle workspaces consume nothing.
- **At scale:** lower the idle timeout toward per-run ephemeral behavior — same mechanism.
- **Enterprise:** a **dedicated worker pool / node per org** (the org is the physical unit, so
  dedication is per-org). Premium isolation tier.

Under `single` mode this collapses to today's shared engine container with the one mount.

---

## 5. Data-at-rest isolation — per-store hybrid

The four stores have different sensitivity and scaling shapes, so they use different
strategies. Every strategy **fails safe**.

### Postgres — shared, `org_id`-scoped, RLS-enforced
Holds lower-sensitivity metadata (conversations, messages, MCP configs, users, invites). Keep
one database, but enforce `org_id` **two ways**:
1. **Single repository chokepoint** injects `org_id` into every query (extends the existing
   repository layer that already threads `WorkspaceID`).
2. **Postgres Row-Level Security** as defense-in-depth: even a hand-written query that forgets
   the filter cannot cross orgs. This directly answers the "one missed `WHERE` = breach" risk.

### Qdrant — shared deployment, mandatory indexed `org_id` partition
**One** Qdrant deployment with an **indexed `org_id` partition field** on every vector — *not*
a collection-per-org (collection-per-tenant does not scale past a few hundred tenants; a
single collection with an indexed tenant field is Qdrant's own multitenancy guidance). This
extends the indexer's existing filter chokepoint (which already does per-repo / per-user
repo-gating). Search scoping:
- Every query carries a **mandatory `org_id`** partition filter (enforced at the chokepoint,
  never client-supplied trust).
- Plus an **optional `workspace_id`** filter. Present = workspace-scoped (default); absent =
  org-wide **cross-workspace search** (§3). The `org_id` partition is *always* applied, so
  cross-workspace never becomes cross-org.

### Memory + code/session storage — physically per-org
The raw source code, the agent's memory, and session transcripts are the crown jewels and are
**physically separated per org** — a dedicated per-org volume / subtree (the same volume §4
scope-mounts into the worker). Within the org's tree, existing `user/` `repo/` `global/`
sub-scoping is retained, plus a workspace dimension.

| Store | Strategy | Enforcement |
|---|---|---|
| Postgres | Shared DB | `org_id` chokepoint **+ RLS** |
| Qdrant | Shared deployment | Mandatory indexed `org_id` partition + optional `workspace_id` |
| Memory | Physically per-org tree | Per-org volume/subtree |
| Code + session + attachments | Physically per-org volume | Per-turn scoped mount (§4) |

---

## 6. Web egress

`WebFetch`/`WebSearch` are the main data-exfiltration vector under a read-only agent (e.g. a
prompt-injection payload in indexed code steering the agent to `evil.com/?leak=…`).

- **`multi` mode:** strip `WebFetch`/`WebSearch` from `AllowedToolsForRole` (one flag-gated
  change). The core promise — "ask about *your* code" — does not depend on live web, and this
  deletes an entire exfil class for near-zero build cost.
- **`single` mode:** web tools remain enabled (trusted deployment).
- **Later / enterprise upgrade (deferred):** an **egress allowlist proxy** that permits vetted
  destinations and blocks arbitrary hosts, sold as a premium feature. Its own project — not in
  this spec.

---

## 7. Secrets — per-org envelope encryption

Per-org secrets (each member's GitHub token, org MCP-server headers, future API keys) use
**envelope encryption**:
- Each org has its own **data-encryption key (DEK)**, wrapped by a **KMS master key**. The
  existing AES-GCM encrypt/decrypt is retained — only the key it uses changes from one global
  key to the per-org DEK.
- **Per-org blast radius:** a leaked/rotated key affects one org, not the platform.
- **Crypto-shredding on org deletion:** destroy the org's DEK and all its ciphertext (tokens,
  MCP headers, attachments if extended) is instantly unrecoverable — making "delete my org's
  data" / erasure claims provable rather than best-effort.
- **`single` mode degrades cleanly:** no KMS → fall back to today's single service key.

---

## 8. Enforcement summary (the invariants)

1. **No worker process ever has two orgs' data in view at the same instant** (§4).
2. **Every Postgres query and Qdrant search carries a server-injected `org_id`**; RLS + the
   partition field make omission fail safe, not leak (§5). Client-supplied identity is never
   trusted — the backend strips and re-stamps it (extends today's `X-Internal-Token` /
   repo-gating pattern).
3. **`org_id` is always applied; `workspace_id` is optional** — cross-workspace search stays
   within the org boundary by construction (§5).
4. **Each org's secrets are encrypted under its own DEK**; deletion is crypto-shredding (§7).
5. **`multi` strips web egress**; `single` keeps it (§6).
6. **All of the above are one flag** — self-hosted runs the `single` settings unchanged (§2).

---

## 9. Deferred to their own spec → plan cycles

Explicitly **out of scope** here (each needs its own design):
- **Onboarding & workspace lifecycle** — signup, personal-org auto-provisioning UX, workspace
  creation, connecting repos, invites/roster generalization.
- **Billing & metering** — plans, usage limits, cost attribution per org.
- **Ops & compliance** — audit logging, tenant data export, KMS/key-rotation operations,
  the egress allowlist proxy build.
- **RBAC beyond admin/member** — richer roles, per-workspace permissions.

---

## 10. Impacted surfaces (orientation for the implementation plan)

Not a task list — a map of where `multi` mode lands, to inform the plan:
- `backend/internal/config` — `DEXIASK_TENANCY_MODE`; de-pin `FixedWorkspaceID`; introduce Org.
- `backend/internal/model` + `repository` — Org entity; `org_id` on scoped rows; repository
  chokepoint; RLS migrations.
- `backend/internal/auth` — resolve `Principal` → org (personal-org auto-provision); org-scoped
  admin bootstrap generalizing today's first-user rule.
- `backend/internal/service/chat_service.go` — stamp `org_id`; `multi` `AllowedToolsForRole`
  (drop web tools); route turns to the shared worker pool with a per-org scoped mount.
- `backend/internal/agent` — Job carries `org_id`; per-turn scoped-mount contract.
- `engine` — shared stateless worker pool; per-turn scoped mount; session transcripts on the
  per-org volume.
- `indexer` — `org_id` as mandatory Qdrant partition; optional `workspace_id`; extend the
  access chokepoint.
- `memory` — per-org tree; workspace dimension.
- Secrets — per-org DEK + KMS wrapping around the existing AES-GCM path.
- `docker-compose.yml` / deploy — `single` unchanged; `multi` = shared worker pool + per-org
  volumes + KMS + RLS-enabled Postgres.
