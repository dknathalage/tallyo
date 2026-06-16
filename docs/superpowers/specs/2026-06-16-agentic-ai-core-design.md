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
| `tools.go` | Tool registry. Each tool = `{name, description, inputSchema, risk: read\|risky, render: table\|card\|summary, handler}`. Handlers call **services only**. | `internal/service/*` |
| `plan.go` | Plan phase: first turn forced via `tool_choice` to call `propose_plan`; returns ordered steps; persisted + streamed before any execution. | sdk, store |
| `permission.go` | Risk gate: on a `risky` tool call, block the loop, emit `access_request`, await an allow/deny decision via a channel, then resume. | store, realtime |
| `checkpoint.go` | Opens an `agent_checkpoint` per execute phase; records each mutation's audit change; performs atomic per-turn revert (inverse changes). | repository, audit, db |
| `context.go` | Builds the model request: stable cached prefix (system + tools), compaction config, context-editing config; maps stored history → model window. | sdk |
| `store.go` | sqlc repo over agent tables; tenant-scoped; mutations via `audit.WithTx`. | repository, db |
| `stream.go` | Publishes agent events over the existing realtime hub. | `internal/realtime` |
| `budget.go` | Per-user rate limit + per-tenant daily token budget; hard-stop enforcement. | store, db |

HTTP handlers live in `internal/http/agent.go` (handlers call the agent service, per layering).

## 5. Plan → execute flow

1. **Send.** `POST /api/agent/conversations/{id}/messages` with the user text. Auth + tenant
   from the existing session guard. Budget check first (§9); hard-stop if exceeded.
2. **Plan phase.** Model called with `tool_choice = {type: "tool", name: "propose_plan"}`
   (forced). `propose_plan` input schema: `{steps: [{tool, summary, risk}]}`. The plan is
   persisted (`agent_step` rows) and streamed (`plan` event). No mutations yet.
3. **Execute phase.** Manual loop, bounded by `maxIterations` (default 25 → rule 2):
   - Open an `agent_checkpoint` for this turn.
   - `read` tool calls run immediately; result streamed (`tool_result` event) and fed back.
   - First `risky` tool call → pause; emit `access_request` (step id + human summary);
     block on a Go channel. `POST /api/agent/steps/{id}/decision` (`allow`/`deny`) unblocks.
     - allow → run service call (audited, SSE), record change under checkpoint, feed result.
     - deny → feed an `is_error` tool_result ("user denied"); model adapts or stops.
   - Loop until `stop_reason != tool_use`.
4. **Finalize.** Stream final assistant text. If the turn mutated, the checkpoint is
   non-empty and the turn is shown with a **Revert** affordance.

Model config: `claude-opus-4-8`, adaptive thinking (`display: summarized` so progress is
visible), `effort: high`, streaming (`max_tokens` ~64000). `ANTHROPIC_API_KEY` via env or a
new flag; if unset, the agent UI shows a disabled banner and the endpoints return a clear
"AI not configured" error.

## 6. Tool surface (v1)

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

- `agent_conversation` — `id, tenant_id, user_id, title, created_at, updated_at, archived_at`.
- `agent_message` — `id, conversation_id, tenant_id, role (user|assistant|system), content (json), token_usage (json), created_at`.
- `agent_step` — `id, message_id, tenant_id, ordinal, tool_name, summary, risk, status (planned|awaiting|allowed|denied|done|error), result (json), created_at`.
- `agent_checkpoint` — `id, message_id, tenant_id, status (open|committed|reverted), created_at, reverted_at`.
- `agent_checkpoint_change` — `id, checkpoint_id, tenant_id, ordinal, entity_type, entity_id, op (create|update), before (json), after (json)`. Mirrors audit change capture; the inverse-apply source for revert.

## 8. Context management

- **Full history** lives in SQLite (UI + revert source of truth).
- **Model window** is built per turn by `context.go`, NOT a raw replay:
  - Stable prefix (system prompt + tool definitions) carries `cache_control` (prompt caching).
  - **Compaction** enabled (beta `compact-2026-01-12`): API summarizes older turns near the
    threshold. We append `response.content` (including compaction blocks) back each turn —
    losing them silently drops compaction state.
  - **Context editing**: clear stale `tool_result` blocks (large read dumps) after they're
    consumed, so the window stays lean without summarizing structure.
- Verify cache hits via `usage.cache_read_input_tokens`; keep volatile content after the
  last cache breakpoint.

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
- Each mutating tool call records an `agent_checkpoint_change` (entity, op, before/after),
  derived from the same change data `audit.Changes` already captures.
- **Revert** (`POST /api/agent/checkpoints/{id}/revert`): in a single tx, apply the inverse
  of each change in reverse ordinal order — `create→delete`, `update→restore before`,
  `record payment→remove`. The revert is itself audited and marks the checkpoint `reverted`
  (so a revert is traceable; re-running the turn is a fresh checkpoint).
- UI lists exactly what a revert will undo before the user confirms.
- Conflict handling: if an entity changed after the checkpoint (edited outside the agent),
  the revert surfaces the conflict and asks the user to confirm or skip that change rather
  than blindly overwriting.

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

## 12. Streaming events (over existing realtime hub)

`thinking` (summary), `plan`, `step_start`, `tool_result`, `access_request`, `decision`,
`message_final`, `error`, `budget_exceeded`. SPA refetches/renders into runes as today.

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
  unavailable in the pinned SDK version.
- Revert of an `update` assumes `before` is fully captured; verify audit change capture
  includes complete prior row state for every v1 risky tool.
- Interrupt (`Esc`) semantics mid-tool-call: cancel before the next tool starts; a tool
  already committed stays committed (revert is the undo path).
