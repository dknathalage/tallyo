# Agentic AI Core — Phase 2 (Frontend Chat) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the chat-first SvelteKit UI for the agent: a conversation home at `/` that sends messages, renders the plan, streams tool results as rich tables/cards, prompts for approval on risky steps, lets the user revert a turn, and degrades cleanly when the agent is disabled.

**Architecture:** A typed agent API client (`api/agent.ts`) + a per-conversation SSE client (`agent/stream.ts`) feed a Svelte 5 runes store (`stores/agentChat.svelte.ts`) that reduces agent events into chat state. Components under `components/agent/` render messages, the plan card, render-hint-dispatched tool results, the access-request prompt, and the revert control. `/` becomes the chat home; existing domain pages stay in nav. **Event-level streaming** (matches the backend: plan → tool results → whole final message); no token-by-token (deferred). Backend is unchanged by this plan.

**Tech Stack:** SvelteKit + adapter-static SPA, Svelte 5 runes, Tailwind CSS 4, Vitest (logic), svelte-check (0 errors / 0 warnings). Build via `npm run build` → `web/build` (embedded by Go).

**Spec:** `docs/superpowers/specs/2026-06-16-agentic-ai-core-design.md` (§11 frontend, §12 events). **Phase 1 backend** is merged on `main`.

## Backend contract (already shipped — do not change)

Endpoints (auth-gated, tenant-scoped; 503 `{"error":"AI not configured"}` when key unset):
- `POST /api/agent/conversations` → conversation `{id, title, ...}`
- `GET /api/agent/conversations` → conversation list
- `GET /api/agent/conversations/{id}/messages` → message history (source of truth)
- `POST /api/agent/conversations/{id}/messages` `{text}` → **202** (runs async; watch the stream)
- `POST /api/agent/steps/{id}/decision` `{decision:"allow"|"deny"}` → **202** (resumes async)
- `POST /api/agent/checkpoints/{id}/revert` → `{conflicts:[...]}` (synchronous)
- `GET /api/agent/conversations/{id}/stream` → SSE; each frame is `data: {"type":...,"data":...}`. **Casing is lowercase `type`/`data`** — confirmed: `agent.Event` has json tags `json:"type"` / `json:"data"` (`internal/agent/stream.go`). Encode the parser to lowercase keys.

**SSE event shapes** (`Data` payloads):
| `Type` | `Data` |
|---|---|
| `plan` | `[{tool, summary, risk}]` |
| `tool_result` | `{toolUseId, render, result, isError}` (success) or `{toolUseId, error, isError:true}` |
| `access_request` | `{stepId, toolName, toolUseId, summary, input, expiresAt}` |
| `message_final` | `string` (the whole assistant reply) |
| `error` | `string` |
| `budget_exceeded` | `string` |
| `step_expired` | `{stepId, toolName}` |

All keys above are **lowercase** (the Go structs carry json tags). `stepId` is a number. The 202 responses for send/decision also include a `{status:"accepted", ...}` body — ignore it.

The per-conversation stream has **no replay** — on (re)connect, refetch `GET .../messages` to reconcile.

## Existing patterns to follow

- `web/src/lib/api/client.ts` — `apiGet/apiPost`, `ApiError{status, body, details}`. Reuse; don't re-implement fetch.
- `web/src/lib/realtime/events.ts` — the singleton `/api/events` EventSource. The agent stream is **separate and per-conversation** — a new client, NOT this singleton, but mirror its lazy-open/cleanup discipline.
- `web/src/lib/stores/*.svelte.ts` + `collection.svelte.ts` — runes store conventions.
- `web/src/lib/api/types.ts` — add agent types here or in `api/agent.ts`.
- `web/src/routes/+layout.svelte` — nav (currently has uncommitted edits; read current state, integrate, don't clobber).

## File Structure

| File | Responsibility |
|---|---|
| `web/src/lib/api/agent.ts` | Typed agent API calls + DTO types. |
| `web/src/lib/agent/stream.ts` | Per-conversation SSE client; typed `AgentEvent` union + frame parser. |
| `web/src/lib/agent/events.ts` | `AgentEvent` type definitions (shared by stream + store). |
| `web/src/lib/stores/agentChat.svelte.ts` | Runes store: conversation, messages, live turn state; reduces events; send/decide/revert actions. |
| `web/src/lib/components/agent/ChatPane.svelte` | Conversation view: message list + live turn + composer. |
| `web/src/lib/components/agent/Composer.svelte` | Input textarea + send; `⌘↵` send. |
| `web/src/lib/components/agent/MessageBubble.svelte` | One user/assistant message. |
| `web/src/lib/components/agent/PlanCard.svelte` | Ordered plan steps with risk badges. |
| `web/src/lib/components/agent/ToolResultView.svelte` | Dispatch on `render` → table/card/summary. |
| `web/src/lib/components/agent/ResultTable.svelte` / `ResultCard.svelte` / `ResultSummary.svelte` | Renderers. |
| `web/src/lib/components/agent/AccessRequestPrompt.svelte` | Allow/Deny a risky step. |
| `web/src/lib/components/agent/RevertControl.svelte` | Revert a turn; show conflicts. |
| `web/src/lib/components/agent/ConversationList.svelte` | Sidebar: list + new chat. |
| `web/src/routes/+page.svelte` | Becomes the chat home. |
| `web/src/routes/dashboard/+page.svelte` | Relocated prior landing content (if any). |
| `web/src/routes/+layout.svelte` | Nav: chat home + domain links; global shortcuts. |

## Conventions
- Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`). No legacy stores syntax for new code.
- `npm run check` must be 0/0 (errors/warnings); `// svelte-ignore`/`any`/`@ts-ignore` need an inline justification.
- Vitest for pure logic (parser, reducer, api client); component tests where they assert real behavior.
- Svelte auto-escapes text — render untrusted record text as `{text}`, never `{@html}`.
- Run from `web/`: `npm run check`, `npx vitest run`, `npm run build`.

---

### Task 1: Agent API client + types

**Files:** Create `web/src/lib/api/agent.ts`; Test `web/src/lib/api/agent.test.ts`

- [ ] **Step 1: Write the failing test** — mock `fetch` (or the `apiPost`/`apiGet` layer) and assert each function calls the right method+path with the right body and returns the parsed value: `createConversation()` → POST `/api/agent/conversations`; `listConversations()` → GET; `listMessages(id)` → GET `/api/agent/conversations/${id}/messages`; `sendMessage(id, text)` → POST `.../messages` `{text}`; `decide(stepId, decision)` → POST `/api/agent/steps/${stepId}/decision` `{decision}`; `revert(checkpointId)` → POST `/api/agent/checkpoints/${id}/revert`. Assert a 503 surfaces as an `ApiError` the caller can detect (status 503).
- [ ] **Step 2: Run** `npx vitest run src/lib/api/agent.test.ts` → FAIL.
- [ ] **Step 3: Implement `api/agent.ts`** using `apiGet/apiPost` from `./client`. Define DTO types: `AgentConversation{id, title, createdAt, updatedAt}`, `AgentMessageDTO{id, role, content, createdAt}` (content is the raw block array — define a minimal `AgentBlock` type for rendering: `{type, text?, toolName?, toolUseId?, input?}`), `PlanStepDTO{tool, summary, risk}`, `RevertResult{conflicts: {table, pk}[]}`. Keep functions thin. **Note:** `apiGet`/`apiPost` return `Promise<T | null>` (null on 401-redirect/204) — declare the function return types accordingly (`Promise<X | null>`) or coalesce, so `npm run check` stays 0/0 under strict mode; tests must handle the `| null`.
- [ ] **Step 4: Run** → PASS. `npm run check` 0/0.
- [ ] **Step 5: Commit** `feat(web): agent API client + types`

---

### Task 2: Typed SSE event union + per-conversation stream client

**Files:** Create `web/src/lib/agent/events.ts`, `web/src/lib/agent/stream.ts`; Test `web/src/lib/agent/stream.test.ts`

- [ ] **Step 1: Define `events.ts`** — a discriminated union `AgentEvent` keyed on `type`, one variant per backend event (plan/tool_result/access_request/message_final/error/budget_exceeded/step_expired) with the `Data` shapes from the contract table. Include the success vs error `tool_result` shapes.
- [ ] **Step 2: Write the failing parser test `stream.test.ts`** — a pure `parseAgentFrame(raw: string): AgentEvent | null` that JSON-parses an SSE `data` payload and narrows to `AgentEvent` (returns null on malformed/unknown). Assert: a `plan` frame parses to `{type:'plan', steps:[...]}`; a `tool_result` success frame; a `tool_result` error frame; a `message_final` frame; malformed JSON → null; unknown type → null. **Match the backend's JSON casing** (capitalized `Type`/`Data` unless json tags exist — confirm and encode that in the parser).
- [ ] **Step 3: Run** → FAIL.
- [ ] **Step 4: Implement `stream.ts`** — `parseAgentFrame` (pure, tested) + `openAgentStream(convId, handlers)`: opens `new EventSource('/api/agent/conversations/'+convId+'/stream', {withCredentials:true})`, on `message` runs `parseAgentFrame` and dispatches to a typed `onEvent(ev)` callback, exposes `close()`. Mirror `realtime/events.ts` discipline (guard `window`, clean close). On `open`, call an `onOpen()` hook (the store uses it to refetch messages for reconcile). On `error`, the browser auto-reconnects; surface `onOpen` again for resync.
- [ ] **Step 5: Run** → PASS. `npm run check` 0/0.
- [ ] **Step 6: Commit** `feat(web): typed agent SSE event union + per-conversation stream client`

---

### Task 3: agentChat runes store (the reducer)

**Files:** Create `web/src/lib/stores/agentChat.svelte.ts`; Test `web/src/lib/stores/agentChat.test.ts`

This is the core. State (runes):
- `conversationId: number | null`, `conversations: AgentConversation[]`
- `messages: AgentMessageDTO[]` (persisted history, source of truth)
- `turn: { thinking?: string; plan?: PlanStepDTO[]; toolResults: ToolResultView[]; pendingAccess?: AccessRequest | null; finalText?: string }` — the live, in-flight turn
- `status: 'idle' | 'running' | 'awaiting' | 'error'`, `errorText?: string`, `enabled: boolean`
- Pure reducer `applyEvent(state, ev: AgentEvent)` — testable in isolation:
  - `plan` → set `turn.plan`, status running
  - `tool_result` → push a `ToolResultView{toolUseId, render, result, isError}` (or error) onto `turn.toolResults`
  - `access_request` → set `turn.pendingAccess`, status `awaiting`
  - `step_expired` → clear `pendingAccess` if it matches, note expiry
  - `message_final` → set `turn.finalText`, status idle (then the store refetches messages and folds the turn into history)
  - `error` / `budget_exceeded` → status error + errorText
- Actions: `newConversation()`, `selectConversation(id)`, `loadConversations()`, `send(text)` (opens/ensures stream, POST sendMessage, reset turn), `decide(stepId, allow)` (POST decision), `revert(checkpointId)` (POST revert, surface conflicts), `disconnect()`.

- [ ] **Step 1: Write failing reducer tests** — drive `applyEvent` through a full sequence (plan → tool_result → access_request → (decide) → tool_result → message_final) and assert state transitions at each step; assert `error`/`budget_exceeded` set the error status; assert an unknown/duplicate event is a no-op. Test the pure reducer WITHOUT the network (export `applyEvent` separately).
- [ ] **Step 2: Run** `npx vitest run src/lib/stores/agentChat.test.ts` → FAIL.
- [ ] **Step 3: Implement the store.** Keep `applyEvent` a pure exported function; the store wires `openAgentStream` → `applyEvent` → runes state, and `onOpen`/`message_final` → `listMessages` refetch (reconcile, since the stream has no replay). `send`: ensure a conversation exists (create if null), ensure the stream is open for it, reset `turn`, then `sendMessage` (202). Handle `ApiError` 503 → set `enabled=false` + a clear message; 429 → surface "rate limit reached". `decide` clears `pendingAccess` optimistically and POSTs. `revert` sets a conflicts result for the UI.
- [ ] **Step 4: Run** → PASS. `npm run check` 0/0.
- [ ] **Step 5: Commit** `feat(web): agentChat runes store + event reducer`

---

### Task 4: ConversationList (sidebar + new chat)

**Files:** Create `web/src/lib/components/agent/ConversationList.svelte`; Test optional Vitest/component.

- [ ] **Step 1:** Implement the sidebar: lists `store.conversations` (title + relative time), highlights the active one, a "New chat" button (`store.newConversation()`), click → `store.selectConversation(id)`. Loads via `store.loadConversations()` on mount (`$effect`). Empty state ("No conversations yet").
- [ ] **Step 2:** `npm run check` 0/0; visual sanity.
- [ ] **Step 3: Commit** `feat(web): conversation list sidebar`

---

### Task 5: Composer (input + send + ⌘↵)

**Files:** Create `web/src/lib/components/agent/Composer.svelte`

- [ ] **Step 1:** A growing textarea + Send button. `⌘↵`/`Ctrl↵` sends; Enter alone inserts newline (or sends — pick the conventional chat behavior: Enter sends, Shift+Enter newline — match a familiar chat UX, document the choice). Disabled while `status==='running'` with a subtle indicator. Calls `store.send(text)` and clears. Empty/whitespace input is a no-op.
- [ ] **Step 2:** `npm run check` 0/0.
- [ ] **Step 3: Commit** `feat(web): chat composer with send shortcut`

---

### Task 6: Message list + MessageBubble

**Files:** Create `web/src/lib/components/agent/MessageBubble.svelte`; (list lives in ChatPane, Task 11)

- [ ] **Step 1:** `MessageBubble` renders one persisted message: role-styled (user right/assistant left, or your design), text from the message's text blocks. For assistant messages whose blocks include tool_use, render a compact "used <tool>" affordance (the rich result is shown live in the turn; persisted history can show a summary). Auto-escape text (`{text}`).
- [ ] **Step 2:** `npm run check` 0/0.
- [ ] **Step 3: Commit** `feat(web): message bubble`

---

### Task 7: PlanCard

**Files:** Create `web/src/lib/components/agent/PlanCard.svelte`; Test optional.

- [ ] **Step 1:** Render `turn.plan` as an ordered list of steps: `summary` + a risk badge (`read` neutral, `risky` amber/red, `meta` muted). Header "Plan". This appears before tool execution.
- [ ] **Step 2:** `npm run check` 0/0.
- [ ] **Step 3: Commit** `feat(web): plan card with risk badges`

---

### Task 8: ToolResultView + render-hint renderers

**Files:** Create `ToolResultView.svelte`, `ResultTable.svelte`, `ResultCard.svelte`, `ResultSummary.svelte`; Test `web/src/lib/components/agent/resultRender.test.ts` (a pure helper).

- [ ] **Step 1: Write a failing test for a pure `chooseRenderer(render, result)` helper** — given `render` hint + `result` JSON, returns which renderer + normalized props: `'table'` for an array of objects (derive columns from union of keys); `'card'` for a single object; `'summary'` for a scalar/string; fallback to `summary` for an unknown hint. Test each + the error case (`isError` → an error renderer).
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** `chooseRenderer` + the three renderers. `ResultTable` renders array-of-objects as a table (escaped cells; numeric right-aligned is a nice-to-have). `ResultCard` renders a single object as key/value rows. `ResultSummary` renders a scalar/string. `ToolResultView` dispatches via `chooseRenderer`, shows an error state when `isError`, and labels the tool. All text escaped (untrusted-content safe).
- [ ] **Step 4: Run** → PASS. `npm run check` 0/0.
- [ ] **Step 5: Commit** `feat(web): rich tool-result renderers (table/card/summary)`

---

### Task 9: AccessRequestPrompt

**Files:** Create `web/src/lib/components/agent/AccessRequestPrompt.svelte`

- [ ] **Step 1:** When `turn.pendingAccess` is set, render a prominent prompt: the `summary` ("Approve running create_invoice?"), the concrete `input` (pretty-printed, escaped), Allow/Deny buttons, and the `expiresAt` countdown. Allow → `store.decide(stepId, true)`; Deny → `store.decide(stepId, false)`. Disable buttons after click. If `step_expired` fires for this step, show "expired" and disable.
- [ ] **Step 2:** `npm run check` 0/0.
- [ ] **Step 3: Commit** `feat(web): risky-op access-request prompt`

---

### Task 10: RevertControl

**Files:** Create `web/src/lib/components/agent/RevertControl.svelte`

- [ ] **Step 1:** On a completed turn that mutated (its assistant message has an associated checkpoint — the history/message must expose `checkpointId`; if the messages endpoint doesn't return it, NOTE this as a backend gap and, for v1, surface revert from the live turn's checkpoint if available, or add a `GET` that lists revertible checkpoints — prefer the simplest: confirm whether `listMessages` includes checkpoint info; if not, flag and scope revert to the most recent turn via a dedicated lookup). Render a "Revert this change" affordance; on click confirm, call `store.revert(checkpointId)`, then show the result: success or a list of `conflicts` ("N change(s) skipped — edited since"). Refetch messages after.
- [ ] **Step 2:** `npm run check` 0/0.
- [ ] **Step 3: Commit** `feat(web): revert control with conflict display`

> **CONFIRMED backend gap (resolved in plan review):** `listMessages` does NOT expose a checkpoint id — the `Message` DTO (`internal/agent/store.go`) is `{id, conversationId, tenantId, role, content, tokenUsage, createdAt}`, the checkpoint is opened against the plan message id (`Agent.Start` → `cp.Open(ctx, planMsgID)`) but never surfaced, and there is no checkpoints endpoint. **This task therefore REQUIRES a small backend addition first** — the smallest fix: extend the messages DTO/query to include `checkpointId` + `checkpointStatus` for the assistant/plan message (join `agent_checkpoint` on `message_id`), so the UI knows which turns are revertible and with which checkpoint id. Do that backend change as Step 0 of this task (migration not needed — the table exists; add a query/DTO field + a tenant-scoped getter), with its own Go test, then build the UI. If you prefer, add `GET /api/agent/conversations/{id}/checkpoints` instead — but the DTO field is smaller. Keep the handler→service→repo + tenant-scoping invariants.

---

### Task 11: ChatPane (assembly)

**Files:** Create `web/src/lib/components/agent/ChatPane.svelte`

- [ ] **Step 1:** Compose the conversation view: scrollable message list (`MessageBubble` per persisted message) → then the live `turn` region (PlanCard if `turn.plan`; ToolResultView per `turn.toolResults`; AccessRequestPrompt if `turn.pendingAccess`; a "thinking…"/running indicator; `turn.finalText` until it folds into history; error banner if status error) → then `Composer` pinned at the bottom. Auto-scroll to bottom on new content (`$effect`). Show the empty/disabled states.
- [ ] **Step 2:** `npm run check` 0/0.
- [ ] **Step 3: Commit** `feat(web): chat pane assembly`

---

### Task 12: Route wiring — `/` = chat, nav, shortcuts, disabled banner

**Files:** Modify `web/src/routes/+page.svelte` (read current content first), create `web/src/routes/dashboard/+page.svelte` if the old `/` had content worth keeping, modify `web/src/routes/+layout.svelte`.

- [ ] **Step 1: Preserve the old landing.** Read the CURRENT `web/src/routes/+page.svelte` (note: it has uncommitted edits — read the working-tree version). If it has real content (a dashboard/landing), move that content into `web/src/routes/dashboard/+page.svelte` and add a nav link. If it's trivial, skip.
- [ ] **Step 2: Make `/` the chat home.** Replace `+page.svelte` with a layout hosting `ConversationList` + `ChatPane` (sidebar + pane). On mount, load conversations; select the most recent or start a new one.
- [ ] **Step 3: Nav** (`+layout.svelte`, integrate with existing uncommitted edits — don't clobber): ensure the domain pages (invoices, participants, estimates, …) remain reachable; mark the chat as home. Post-login redirect goes to `/`.
- [ ] **Step 4: Global shortcuts** — `⌘K`/`Ctrl-K` new chat, `⌘⇧K` clear current conversation view, `⌘↵` send (in Composer, Task 5), `Esc` dismiss the access prompt / stop watching the stream. **Honesty note:** there is no backend interrupt endpoint in Phase 1 — the agent runs detached server-side, so `Esc` is client-side only (dismiss/stop-watching), NOT a true cancel. Document this in code + surface nothing misleading in the UI. (True interrupt = a follow-up backend endpoint.)
- [ ] **Step 5: Disabled state** — when the API returns 503 (agent not configured), the chat home shows a clear "AI assistant is not configured" banner and disables the composer; the rest of the app (nav, domain pages) works normally.
- [ ] **Step 6:** `npm run check` 0/0; `npm run build` succeeds (emits `web/build`).
- [ ] **Step 7: Commit** `feat(web): chat-first home, nav, shortcuts, disabled banner`

---

### Task 13: End-to-end build verification

- [ ] **Step 1:** From `web/`: `npm run check` (0/0), `npx vitest run` (all green), `npm run build` (emits `web/build`).
- [ ] **Step 2:** From repo root: `CGO_ENABLED=0 go build .` (embeds the new SPA build) and `go test ./... -race` (backend unaffected).
- [ ] **Step 3: Commit** any build artifacts/config if the repo commits `web/build` (check — it may be gitignored; if so, nothing to commit). `chore(web): rebuild SPA with agent chat`

---

## Definition of done (Phase 2)
- `npm run check` 0/0, Vitest green, `npm run build` succeeds, `CGO_ENABLED=0 go build .` embeds it.
- With the agent enabled: `/` opens the chat; sending "list my invoices" shows a plan then a tool-result table then the final reply; "create an invoice…" shows a plan, an access prompt, and on Allow the result + a revert affordance; Deny cancels cleanly.
- With the agent disabled (no key): `/` shows the "not configured" banner; the rest of the app works.
- Untrusted record text is escaped (no `{@html}` on agent data).

## Notes / honest scope
- **Event-level streaming only** — the final assistant message appears whole (`message_final`); token-by-token is a separate backend+frontend follow-up.
- **`Esc` is not a true interrupt** in Phase 1 (no backend cancel endpoint); it dismisses/stops watching client-side.
- **Revert checkpoint exposure** (Task 10) may require a small backend addition if `listMessages` doesn't surface a turn's checkpoint id — flagged as a pre-task check.
- The uncommitted `web/` working-tree changes present at plan time are unrelated to this work; reconcile them before/independently of this plan.

## Follow-on (not this plan)
- Token-level streaming (backend text deltas + UI typing).
- True interrupt endpoint + `Esc` wiring.
- Cross-session memory UI; conversation rename/archive/delete.
