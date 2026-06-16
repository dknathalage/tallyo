# Agentic AI Core — Design

**Date:** 2026-06-16
**Status:** Draft (design approved in brainstorming; pending spec review + implementation plan)

## 1. Goal

Put an agentic AI assistant at the centre of Tallyo. The user issues a natural-language
command ("invoice John for last week's sessions", "chase overdue NDIS claims"); the agent
**plans** an ordered set of steps, shows the plan, then **executes** it — calling the
existing service layer. Risky (mutating) operations pause and request the user's access
before running. The app's home becomes a conversation.

Non-goals for v1: autonomous/scheduled operation, read-only "advisor" dashboards as the
primary surface, data-entry copilot parsing, cross-tenant or admin operations, deletes.

## 2. Decisions (locked in brainstorming)

| Decision | Choice |
|---|---|
| Core job | Command-driven plan→execute over existing services |
| LLM | Anthropic Claude `claude-opus-4-8`, `anthropic-sdk-go`, **manual** agentic loop |
| Thinking / effort | `thinking: {type: "adaptive"}`, `effort: "high"`, streaming |
| Plan style | Explicit two-phase: structured plan first, then execute |
| Gate | Reads auto-run; risky mutations pause and request access |
| Surface | Chat-first home (conversation is the primary view) |
| Tool scope (v1) | Focused: all-domain reads + invoice/estimate/payment/participant writes + sweeps. No deletes. |
| Memory | Per-conversation (full history persisted; compacted window sent to model). No cross-session memory. |
| Context mgmt | Server-side compaction + context editing + prompt caching |
| Revert | Per agent turn (= a git-like commit); DB-level, trigger-driven `row_change` journal; reverse-diff (column-level, `git revert`-style) replay; append-only + re-revertible |
| Budget cap | Hard stop + clear notice when tenant hits daily token ceiling |

## 3. Why this architecture

Chosen: a new `internal/agent` package running a **manual** Claude agentic loop, with
tools that wrap the existing **service** layer.

Rejected alternatives:

- **Anthropic Managed Agents (hosted loop + container).** The hosted container cannot
  reach the local SQLite DB / service layer of a self-hosted single binary. Wrong fit.
- **`BetaToolRunner` auto-loop.** The runner executes tools for us; injecting a mid-loop
  "request access" gate fights the abstraction. The manual loop
  (`client.Messages.New` + `StopReasonToolUse`) gives the control the permission model needs.

Invariants preserved (CLAUDE.md): handler→service→repo layering (the agent is just another
service caller), every mutation audited via `audit.WithTx`, SSE broadcast after commit,
tenant scoping, cgo-free build (`anthropic-sdk-go` is pure Go).

## 4. Components — `internal/agent/`

Each unit has one purpose, a clear interface, and is testable in isolation.

| Unit | Purpose | Depends on |
|---|---|---|
| `agent.go` | Orchestrator: runs plan phase then execute loop; owns per-message iteration cap. | sdk, registry, store, permission |
| `tools.go` | Tool registry. Each tool = `{name, description, inputSchema, risk: read\|risky\|meta, render: table\|card\|summary, handler}`. Domain handlers call **services only**; `meta` tools (e.g. `propose_plan`) perform no service call and are neither gated nor audited. | `internal/service/*` |
| `plan.go` | Plan phase: first turn forced via `tool_choice` to call `propose_plan`; returns ordered steps; persisted + streamed before any execution. | sdk, store |
| `permission.go` | Risk gate: on a `risky` tool call, suspend the loop (persist `agent_step.status = awaiting` + pending tool_use), emit `access_request`, and return; a later decision request resumes from persisted state (§5a). No in-memory block. | store, realtime |
| `checkpoint.go` | Opens an `agent_checkpoint` per execute phase, stamps the per-tx checkpoint tag so the DB-level journal (§7, §10) attributes writes to it; performs atomic per-turn revert by replaying the journal's generic inverse. | db, repository |
| `context.go` | Builds the model request: stable cached prefix (system + tools), compaction config, context-editing config; maps stored history → model window. | sdk |
| `store.go` | sqlc repo over agent tables; tenant-scoped; mutations via `audit.WithTx`. | repository, db |
| `stream.go` | Publishes agent events over the existing realtime hub. | `internal/realtime` |
| `budget.go` | Per-user rate limit + per-tenant daily token budget; hard-stop enforcement. | store, db |

HTTP handlers live in `internal/http/agent.go` (handlers call the agent service, per layering).

## 5. Plan → execute flow

1. **Send.** `POST /api/agent/conversations/{id}/messages` with the user text. Auth + tenant
   from the existing session guard. Budget pre-check (§9); hard-stop if already exceeded.
   Budget is **also re-checked between loop iterations** using usage returned on each model
   response — if the running daily total crosses the ceiling mid-turn, the loop stops cleanly
   after the current iteration, emits `budget_exceeded`, and commits any checkpoint changes so
   far (no partial silent overshoot beyond one in-flight model call).
2. **Plan phase.** Model called with `tool_choice = {type: "tool", name: "propose_plan"}`
   (forced). `propose_plan` is a registered **meta-tool** (§6) — `risk: meta`, no service
   call, not audited, never gated. Input schema: `{steps: [{tool, summary, risk}]}`. The plan
   is persisted as `agent_step` rows (`status = planned`) and streamed (`plan` event). No
   mutations yet.

   **Plan vs. execute steps.** Planned rows are advisory, for display. The execute loop is
   authoritative: each actual tool call (planned or not — the model may read tools it didn't
   plan, or deviate after a deny) creates/updates an `agent_step` row keyed by its
   `tool_use_id`. A planned row is matched to its execution by tool_use_id when the model
   follows the plan, or left `planned` (unexecuted) when the model deviates. All steps for a
   turn carry the **assistant `message_id` of that turn's first assistant message** (the plan
   message); the loop's intermediate model turns are stored as `agent_message` rows but steps
   belong to the turn, not each intermediate message.
3. **Execute phase.** A **resumable** manual loop (see §5a), bounded by `maxIterations`
   (default 25 → rule 2):
   - Open an `agent_checkpoint` for this turn.
   - `read` tool calls run immediately; result streamed (`tool_result` event) and fed back.
   - A `risky` tool call → the loop **suspends**: persist its state, set the step
     `status = awaiting`, emit `access_request` (step id + human summary), and **return the
     HTTP request**. Nothing blocks a goroutine waiting for a human.
   - `POST /api/agent/steps/{id}/decision` (`allow`/`deny`) **resumes** the loop from
     persisted state (§5a):
     - allow → run service call (audited, SSE), record change under checkpoint, feed result,
       continue the loop.
     - deny → feed an `is_error` tool_result ("user denied"); model adapts or stops.
   - Loop until `stop_reason != tool_use`.
4. **Finalize.** Stream final assistant text. If the turn mutated, the checkpoint is
   committed (non-empty) and the turn is shown with a **Revert** affordance.

## 5a. Resumable loop & gate mechanism

The gate spans two HTTP requests (the message request that proposes a risky step; the
decision request that answers it). **No in-memory channel block.** Instead:

- **Loop state is persisted, not held in a goroutine.** The model conversation window is
  reconstructable from stored `agent_message` content blocks (§8), so the loop can stop and
  resume. The message request runs the loop until it either finishes or hits a risky step;
  on a risky step it persists `agent_step.status = awaiting` (with the pending `tool_use`
  block + tool_use_id stored on the step) and returns.
- **Resume** is driven by the decision request: it loads the conversation, appends the
  `tool_result` (allow→service result, deny→error), and re-enters the same loop function.
  The loop function is pure over (stored history → next action), so message-start and
  resume share one code path.
- **Streaming during resume**: the decision request opens the agent SSE stream (§12) for the
  continuation, exactly as the message request does.
- **Never-answered steps (timeout/cleanup).** An `awaiting` step has a TTL (default 30 min).
  The existing hourly sweep (extended in `main.go`) reaps expired `awaiting` steps: mark the
  step `denied (expired)`, mark the turn's `open` checkpoint `committed` (prior allowed
  changes in the turn stand and remain revertible), emit a `step_expired` event. A user can
  also explicitly cancel (`Esc`, §11) which denies the awaiting step immediately.
- **Concurrency**: at most one `awaiting` step per conversation; a decision is idempotent
  (a second decision for an already-resolved step is a no-op + clear error).

Model config: `claude-opus-4-8`, adaptive thinking (`display: summarized` so progress is
visible), `effort: high`, streaming (`max_tokens` ~64000). `ANTHROPIC_API_KEY` via env or a
new flag; if unset, the agent UI shows a disabled banner and the endpoints return a clear
"AI not configured" error.

## 6. Tool surface (v1)

**Meta (control-plane, not gated/audited):** `propose_plan`.

**Read (auto-run):** list/get for participants, invoices, estimates, payments, plan
managers, support catalogue; cashflow / overdue summaries.

**Risky (gated):** create/update invoice, create/update estimate, mark estimate accepted,
record payment, create participant, run recurring sweep, run overdue sweep.

**Excluded from v1:** all deletes, bulk/destructive ops, tenant-config changes,
plan-manager edits.

Each tool: validates inputs at the boundary (≥2 checks, rule 5); enum-constrained where a
fixed value set exists; returns structured JSON + a `render` hint.

## 7. Data model (new goose migration `NNNNN_agent.sql`)

All tables tenant-scoped (`tenant_id`), all mutations via `audit.WithTx`, list queries
return `[]` non-nil. sqlc source in `internal/db/queries/agent.sql`.

- `agent_conversation` — `id, tenant_id, user_id, title, compacted_through_message_id (nullable — boundary the latest compaction block subsumes, see §8), created_at, updated_at, archived_at`.
- `agent_message` — `id, conversation_id, tenant_id, role (user|assistant), content (json — stores the raw SDK content blocks verbatim, including thinking and compaction blocks, see §8), token_usage (json), created_at`. (No `system` role: the system prompt is the cached prefix built by `context.go`, never a stored message.)
- `agent_step` — `id, message_id, tenant_id, ordinal, tool_name, summary, risk, status (planned|awaiting|allowed|denied|done|error), result (json), created_at`.
- `agent_checkpoint` — `id, message_id, tenant_id, status (open|committed|reverted), created_at, reverted_at`. The checkpoint `id` **is** the journal `txn_tag` (§10).

**DB-level reversibility (not agent-specific; lives in `internal/db`):**

- `row_change` — the generic, DB-wide change journal, populated by triggers (§10), **not**
  by application code: `seq (single INTEGER PRIMARY KEY AUTOINCREMENT — globally monotonic
  across all tables; revert orders by this), table_name, pk, tenant_id (nullable), op
  (insert|update|delete), old_row (json — null for insert), new_row (json — null for delete),
  txn_tag (nullable — the agent_checkpoint id when the write happened inside a tagged span,
  else null), actor_user_id, created_at`. Single source of truth for revert; replaces the
  previous per-entity `agent_checkpoint_change` table. Exempt from the audit invariant (§10).

## 8. Context management

**Source of truth.** `agent_message.content` stores the **raw SDK content blocks verbatim**
for every turn — user blocks, assistant text/thinking blocks, `tool_use`/`tool_result`
blocks, and any server-side **compaction blocks**. This is what makes the loop resumable
(§5a) and what preserves compaction state across turns.

**Building the model window** (`context.go`) — a faithful replay of stored blocks, made lean,
not a lossy paraphrase:

- **Replay**: map stored `agent_message.content` rows directly to SDK `MessageParam` blocks,
  in order. Because compaction blocks were stored verbatim, appending them back each turn
  preserves compaction state (dropping them would silently lose it — the one hard rule).
- **Compaction** (beta `compact-2026-01-12`, Opus 4.8): the API summarizes older turns near
  the threshold and returns a compaction block in `response.content`. We persist that block
  with the assistant message **and** record the boundary it subsumes in
  `agent_conversation.compacted_through_message_id`. **Replay rule** (applies equally to a
  fresh turn and a §5a resume, so the resumable-loop guarantee and compaction stay
  consistent): replay messages with id `>` `compacted_through_message_id` verbatim, prefixed
  by the stored compaction block; never replay the raw rows the compaction block already
  subsumes. A later compaction advances the marker and supersedes the prior block. This is
  the single boundary that prevents duplicated/stale history on resume.
- **Context editing**: large, already-consumed `tool_result` blocks (read dumps) are cleared
  from the *replayed window* (kept in SQLite for the UI). Keeps the window lean without
  paraphrasing structure.
- **Caching**: stable prefix (system prompt + tool definitions) carries `cache_control`;
  volatile content sits after the last breakpoint. Verify hits via
  `usage.cache_read_input_tokens`.

So "not a raw replay" means *stale tool dumps are pruned and old turns are compacted* — the
blocks that remain are replayed verbatim, never reconstructed.

## 9. Guardrails

- **No escape-hatch tools.** Only whitelisted typed tools — no bash, SQL, or arbitrary
  code. The model cannot act outside the registry.
- **Tenant confinement.** Every tool handler scopes to the caller's tenant via the existing
  service/repo tenant scoping. Cross-tenant access is impossible at the data layer.
- **Prompt-injection defense.** Untrusted record content (participant notes, imported CSV,
  descriptions) is wrapped/labelled as data, never instructions; the system prompt instructs
  the model to treat record text as untrusted and never follow instructions embedded in it.
  Risky ops gate regardless of what injected text requests.
- **Abuse limits (`budget.go`).** Per-user message rate limit; per-tenant **daily token
  budget** with a **hard stop** + clear "daily AI limit reached" notice on exceed; per-message
  `maxIterations` and max-tool-calls caps (rule 2). Auth required (session guard).
- **Audit.** Every agent action, access decision, and revert is logged via `audit.WithTx`.

## 10. Revert / versioning

Reversibility is a property of the **data layer**, not reconstructed in services/handlers.
The model is **git-like**: checkpoints are commits on a per-conversation timeline, history is
append-only, and revert is a forward operation (like `git revert`, not `git reset`) that is
itself journaled and therefore re-revertible ("un-revert" = revert the revert span).

**Capture (triggers).** Every mutable domain table gets `AFTER INSERT/UPDATE/DELETE`
triggers that write a `row_change` row: `op`, the full `old_row`/`new_row` as
`json_object(...)` of all columns, the table's `tenant_id` (null for tenant-less tables, see
§14), the actor, and the current `txn_tag`. Triggers are **generated from the schema** and
shipped as goose migrations (regenerated when columns change — a generator + a test asserting
every mutable table has current triggers). This catches every write — including FK cascade
and side-effect writes (e.g. a payment also bumping an invoice's stored balance) — regardless
of which Go code issued it. `row_change` is itself written only by triggers and is **exempt
from the "every mutation audited" invariant** by design (it's infrastructure, not a domain
mutation); the independent `audit_log` is untouched.

**Tagging (lifecycle-safe across the pool).** Tagging is folded into the shared
`audit.WithTx` wrapper so the stamp and the domain mutation always ride the **same** pinned
`*sql.Tx` connection (no separate tx, no layering break): as its **first** statement every
write tx does `DELETE FROM _txn_context` (clearing any stale tag left on a pooled connection),
then, if a checkpoint id is present on the `context.Context`, `INSERT INTO _txn_context`. The
agent sets that context value when running a risky tool; all other callers leave it unset →
`txn_tag` null. Triggers read `(SELECT checkpoint_id FROM _txn_context)`. Because the table is
cleared at the start of *every* write tx, pooled-connection reuse can never carry a stale tag.
`database/sql` pins one connection per `*sql.Tx` and `_txlock=immediate` serializes writers,
so the stamp is isolated to its tx.

**Revert** (`POST /api/agent/checkpoints/{id}/revert`): one generic operation, identical for
every table — no per-entity logic. In a single tx that first sets `PRAGMA defer_foreign_keys
= ON` (so intra-tx FK ordering doesn't matter — restored child/parent rows are checked only at
commit; required because reverse-`seq` order does not respect referential ordering against the
`ON DELETE CASCADE` FKs in the schema). For each `row_change` with `txn_tag = checkpoint_id`
in **reverse `seq`**, apply a **reverse-diff** (column-level, git-style), not a snapshot reset:
- `insert →` `DELETE` by pk.
- `delete →` `INSERT old_row`.
- `update →` compute the columns this change altered (`old_row` vs `new_row`); set **only
  those columns** back to their `old_row` values on the **current** row. Later edits to
  *other* columns of the same row survive — exactly like `git revert`'s 3-way merge.

Cascade/side-effect writes were journaled too, so reverting the span restores payments **and**
the balances they touched atomically — the prior "compensating reversal" special-case is gone.
The revert runs through `audit.WithTx` (logged) and is itself journaled (re-revertible); it
marks the checkpoint `reverted`.

**Conflict detection (per-column, git-like).** For an `update` change, conflict iff a journal
entry on the same `(table_name, pk)` with `seq` greater than this checkpoint's max seq for that
pk **and** `txn_tag != checkpoint_id` (excludes the checkpoint's own multi-write spans and the
in-progress revert) altered **one of the same columns** this revert would touch. Same-column
later edit → surface the conflict (overwrite/skip per row, like git's ours/theirs); edits to
other columns are not conflicts and revert cleanly. For `insert→delete` and `delete→insert`,
conflict iff the row's existence changed again afterward.

**UI** lists exactly what a revert will undo (and any conflicts) before the user confirms.

## 11. Frontend (`web/`)

- **Chat-first home.** App opens to the conversation pane. Classic pages (participants,
  invoices, …) remain browsable; the agent can open them and the user can navigate directly.
- **Rich rendering.** Tool results carry a `render` hint (`table` | `card` | `summary`); the
  SPA renders rich tables/cards, not raw text. A small set of typed renderers keyed by hint.
- **Plan card** shows ordered steps with risk badges before execution.
- **Access-request prompt** for each gated step (allow/deny), with the concrete action.
- **Revert control** on any turn that mutated, listing the changes it will undo.
- **Shortcuts:** `⌘K`/`Ctrl-K` new chat, `⌘⇧K` clear, `⌘↵` send, `Esc` cancel/interrupt a
  running agent.
- svelte-check clean (0/0), Svelte 5 runes, Tailwind 4 (existing conventions).

## 12. Streaming events

Two distinct channels — don't conflate them:

- **Existing realtime hub** (`/api/events`): unchanged. Entity-change broadcasts so the SPA
  refetches lists/cards when the agent's *service calls* mutate domain data (same as any
  other mutation today).
- **Dedicated agent stream** (new, per-conversation SSE, e.g. `/api/agent/conversations/{id}/stream`):
  high-frequency, single-user agent events — token/`thinking` deltas, `plan`, `step_start`,
  `tool_result`, `access_request`, `step_expired`, `message_final`, `error`,
  `budget_exceeded`. This is a per-request stream (interruptible via `Esc`, §11), **not** the
  broadcast hub. The decision/resume request (§5a) attaches to this same stream.

`decision` is **inbound** (`POST .../steps/{id}/decision`), not a streamed event; the
outbound acknowledgement is `step_start`/`tool_result` for the resumed step.

## 13. Testing

- Go stdlib tests per unit: registry validation, plan-phase forced-tool parsing, permission
  gate (allow/deny/timeout), checkpoint revert (create/update/payment inverses + conflict),
  budget hard-stop, tenant confinement (a tool cannot reach another tenant's rows).
- The Claude client is wrapped behind an interface so the loop is testable with a fake that
  scripts tool-call turns — no network in tests.
- Frontend: svelte-check + Vitest for renderers and shortcut handling.

## 14. Risks / open questions

- Compaction/context-editing betas: confirm exact `anthropic-sdk-go` surface during
  implementation; fall back to manual truncation of stale tool_results if a beta is
  unavailable in the pinned SDK version (this is why the window builder treats context
  editing as a prune of replayed blocks, not an SDK-only feature).
- **Trigger generation** (§10): build the schema→trigger generator and the test that asserts
  every mutable table has up-to-date INSERT/UPDATE/DELETE triggers; decide build-time vs
  hand-maintained. Generator must handle **tenant-less tables** (catalog tables keyed by
  `catalog_version_id`, `business_profile`, etc.) by writing `tenant_id = null` rather than
  referencing a missing column. All v1 gated tables (invoices, line_items, estimates,
  estimate_line_items, payments, participants) carry `tenant_id`. (Payment-vs-balance is no
  longer open — the journal captures both writes, so revert restores them together.)
- **Journal retention** (§7): `row_change` grows with every mutation. Prune in the hourly
  sweep, keyed to **checkpoint lifecycle, not age** — never prune rows whose `txn_tag` points
  at a still-`committed` (revertible) checkpoint, or the turn's Revert affordance silently
  breaks. Null-`txn_tag` rows (non-agent writes) have no owning checkpoint → separate
  age-based retention (e.g. keep N days).
- Interrupt (`Esc`) semantics mid-tool-call: cancel before the next tool starts; a tool
  already committed stays committed (revert is the undo path); an `awaiting` step is denied.
- `maxIterations` (25) vs. long plans with many gated steps: confirm the cap counts model
  iterations, not gated waits, so a legitimately long plan isn't truncated by the bound.
