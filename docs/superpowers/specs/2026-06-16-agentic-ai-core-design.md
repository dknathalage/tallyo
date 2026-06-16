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
| Revert | Per agent turn, atomic, via audit-derived inverse changes |
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
| `checkpoint.go` | Opens an `agent_checkpoint` per execute phase; records each mutation's audit change; performs atomic per-turn revert (inverse changes). | repository, audit, db |
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
- `agent_checkpoint` — `id, message_id, tenant_id, status (open|committed|reverted), created_at, reverted_at`.
- `agent_checkpoint_change` — `id, checkpoint_id, tenant_id, ordinal, entity_type, entity_id, op (create|update), before (json — full prior row snapshot, captured by the tool handler before mutating; null for create), after (json — full new row), entity_version (the entity's updated_at/version at mutation time, for conflict detection)`. The full-row `before` is captured by the checkpoint writer itself, **not** derived from `audit.Changes` (which may store only field-level diffs); audit remains the independent immutable log.

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

- One `agent_checkpoint` per execute phase (per-turn granularity).
- Each mutating tool call records an `agent_checkpoint_change` with a **full prior-row
  snapshot** captured by the tool handler immediately before the mutation (§7) — independent
  of `audit.Changes`, so revert never depends on audit storing complete prior state.
- **Revert** (`POST /api/agent/checkpoints/{id}/revert`): in a single tx, apply the inverse
  of each change in reverse ordinal order via the service layer:
  - `create → delete` the created entity.
  - `update → restore` the full `before` snapshot.
  - `record payment →` issue a **compensating reversal/removal** of that payment. This is a
    deliberate, audited exception to the v1 "no delete tools" rule: revert is a system
    operation (not an agent tool), and it must be able to undo a recorded payment. Whether
    that is a hard delete or a reversal entry is decided per-entity to keep invoice balances
    correct (see open question §14).
  - The revert is itself audited via `audit.WithTx` and marks the checkpoint `reverted` (so
    the revert is traceable; re-running the turn produces a fresh checkpoint).
- UI lists exactly what a revert will undo before the user confirms.
- **Conflict detection**: each change stored `entity_version` (updated_at/version) at mutation
  time. On revert, compare the entity's current version; if it changed after the checkpoint
  (edited outside the agent, or by a later agent turn), surface the conflict and let the user
  confirm-overwrite or skip that specific change rather than blindly clobbering.

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
- **Payment revert mechanism** (§10): decide per-entity whether reverting a recorded payment
  is a hard delete or a compensating reversal entry, so invoice balances/aggregates stay
  correct. Resolve before implementing the payment tool's revert path.
- Interrupt (`Esc`) semantics mid-tool-call: cancel before the next tool starts; a tool
  already committed stays committed (revert is the undo path); an `awaiting` step is denied.
- `maxIterations` (25) vs. long plans with many gated steps: confirm the cap counts model
  iterations, not gated waits, so a legitimately long plan isn't truncated by the bound.
