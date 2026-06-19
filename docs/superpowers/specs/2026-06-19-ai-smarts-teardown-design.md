# AI Smarts Teardown — Design

Date: 2026-06-19
Status: Proposed
Related skill: `designing-ai-smarts` (curated button-triggered AI actions; propose → apply)

## Problem

The `draft-invoice` flow is broken: the SPA gets `Unexpected token 'c', "create_inv"... is not valid JSON`. Root cause is structural, not a one-line bug: an autonomous one-shot action ("turn these shifts into an invoice") is forced through a **conversational agent harness** (plan phase → message/step persistence → risky-tool suspend/approve gate → checkpoint revert → SSE chat stream → token budget). `autoApproveInvoice` fakes human approval by polling `ListExpiredAwaitingSteps` with `farFuture="2999-01-01"`, then digs the result out of DB step columns. When `create_invoice` errors, the raw error string (`create_invoice: …`) is stored in the `result` column and returned as the HTTP body because the read checks `ToolName` + non-empty `Result` but never `Status=="done"`.

The deeper finding: **the conversational surface does not exist.** The agent chat endpoints (`/api/agent/conversations`, `/messages`, `/stream`, `/steps/{id}/decision`, `/checkpoints/{id}/revert`) are never called from `web/src`. There is no chat UI, no message thread, no assistant panel. The entire harness exists to serve a chat product that was never built.

## Goal

Replace the agent harness with **Smarts**: a curated set of named, button-triggered AI actions, each `gather → propose → apply`, each returning an editable result. No conversation, no prompt box, no approval gate, no checkpoint machinery. The surfacing bug becomes structurally impossible because errors are plain Go errors returned through the normal error path.

This is a clean-break deletion, consistent with the project's clean-break data-model stance.

## Current state (inventory)

**Used AI features (keep the capability):**
- `POST /api/shifts/import` → `ImportShifts` → `ExtractShifts` (`extract.go`). **Already a correct Smart**: forced-tool LLM call → validate → `shift.Service.Create`. Touches none of the harness. This is the reference implementation.
- `POST /api/participants/{id}/draft-invoice` → `DraftInvoiceFromShifts` → starts the agent, `autoApproveInvoice` hack. **Broken; rebuild as a Smart.**

**Dead AI surface (delete):**
- Endpoints: `/api/agent/conversations` (create/list), `/conversations/{id}/messages` (list/send), `/conversations/{id}/stream`, `/agent/steps/{id}/decision`, `/agent/checkpoints/{id}/revert`.
- Frontend `web/src/lib/api/agent.ts` (+ test) — defined, never imported.

**Harness internals (delete):**
`agent.go` (Start/Execute/plan loop/loadHistory/nudge/stall/persistAssistant), `permission.go`, `plan.go`, `checkpoint.go`, `budget.go`, `store.go` (conversations/messages/steps/checkpoint-change persistence), `stream.go` (incl. the agent-chat `Events`), `sweep.go`, the `Tool`/`Registry`/`Result` machinery in `tools.go`, and the tool wrappers in `tools_invoice.go`/`tools_shifts.go` that exist only to register with the loop.

**Delete *partially* — carve out before deleting (review B1/B2):**
- `context.go` — delete `buildRequest`, but the per-turn token ceiling `requestMaxTokens = 64000` (`context.go:11`) is used by kept `extract.go:96`. **Relocate the const** to a kept file (the new `smarts.go`) before deleting.
- `prompt.go` — delete `SystemPrompt` (the loop's system prompt), but **keep `wrapUntrusted`** (`prompt.go:42`): it's called by kept `extract.go:94` and `tools_shifts.go:113`. Move it to a kept file.
- `agent.go`'s `reservedToolNamePrefix` (used by `tools.go:59`) is moot once `tools.go` is deleted.

**Keep:**
- `internal/agent/llm/` — clean provider adapter (`Client`, `Request` with `Tools`+`ToolChoice.ForceTool`, `Response` with `tool_use` blocks). This is all a Smart needs.
- `extract.go` — already a Smart (depends on `wrapUntrusted` + `requestMaxTokens`, both relocated above).
- The deterministic invoice build, lifted into `applyDraftInvoice`. Review S2 confirms these are pure functions over interfaces with **zero** `Checkpoint`/`Result`/`Tool` entanglement: `verifyShiftsCovered`, `billCoveredShifts`, `coverageRange`, `hasCodedLine`, `codedDateRange`, `round2c` (`tools_invoice.go:298-400`), plus `createInvoiceInput`/`createInvoiceSchema`, `CreateWithCatalogPricing` (in `invoice` slice), and the NDIS `LineValidator` in `billing`. The only checkpoint coupling is the self-contained `checkpointFrom(ctx)` recording block (`tools_invoice.go:268-281`) — dropped.
- **`shiftCandidates` (review S1):** the catalogue-candidate resolution the gather step needs currently lives in `tools_shifts.go:129`, entangled with the `shiftView`/`Tool` wrapper. It must be **lifted out** of `tools_shifts.go` into the Smart's gather, not just the `tools_invoice.go` helpers.
- Handler helpers used by the kept `ImportShifts`: `guard`, `detach` (`agent_handler.go:292,313`), `existingShiftKeys`, `shiftDedupKey`.
- `deps.go` interfaces (trimmed to what Smarts use): `InvoiceCreator`, `ShiftWorker` (`ShiftLister`+`ShiftDrafter`), `CatalogueSearcher`. Drop `InvoiceLister`, `InvoiceAccessor` (revert), and the checkpoint-only paths.

## Target architecture

### The Smart engine

A single generic helper does the LLM half. No loop, no history, no registry object — just a forced structured call.

```go
// propose forces the model to emit exactly one tool_use whose input matches the
// schema, and decodes it into T. One call, no conversation. The model's only job
// is to fill the schema; deterministic Go owns everything after.
func propose[T any](ctx context.Context, c llm.Client, cfg Config,
    system, userContent, toolName string, schema json.RawMessage) (T, error) {

    var zero T
    req := llm.Request{
        System:     system,
        Tools:      []llm.ToolDef{{Name: toolName, InputSchema: schema}},
        ToolChoice: llm.ToolChoice{ForceTool: toolName},
        Messages:   []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{{Type: llm.BlockText, Text: userContent}}}},
        MaxTokens:  requestMaxTokens, Model: cfg.Model, Effort: cfg.Effort,
    }
    resp, err := c.CreateMessage(ctx, req)
    if err != nil { return zero, fmt.Errorf("propose %s: %w", toolName, err) }
    for i := range resp.Content { // bounded by len(Content)
        b := resp.Content[i]
        if b.Type == llm.BlockToolUse && b.ToolName == toolName {
            var out T
            if e := json.Unmarshal(b.Input, &out); e != nil {
                return zero, fmt.Errorf("propose %s: decode: %w", toolName, e)
            }
            return out, nil
        }
    }
    return zero, fmt.Errorf("propose %s: model emitted no %s call", toolName, toolName)
}
```

### A Smart = gather → propose → apply, with bounded retry

Each Smart is a plain method on a `Smarts` service (constructed in `internal/app` from the existing slice services). Shape:

```
gather(ctx)  → pull inputs from app state (shifts, catalogue candidates). No user prompt.
propose(...) → propose[T] with the Smart's schema + system prompt.
apply(...)   → deterministic validate → compute → write via the normal service layer.
               On a *validation* error (recoverable), re-propose feeding the error
               text back into userContent, bounded to maxRetries=2. NOT a conversation.
result       → typed value (invoice/shift list/code), returned as normal JSON.
```

Retry loop (bounded, rule 2):

```go
const maxRetries = 2
var lastErr string
for attempt := 0; attempt <= maxRetries; attempt++ { // bounded
    content := base
    if lastErr != "" { content = base + "\n\nYour previous attempt failed: " + lastErr + "\nFix it." }
    proposal, err := propose[createInvoiceInput](ctx, c, cfg, system, content, "create_invoice", createInvoiceSchema)
    if err != nil { return nil, err } // model/transport failure: not retryable here
    inv, aErr := s.applyDraftInvoice(ctx, proposal) // deterministic
    if aErr == nil { return inv, nil }
    if ve, ok := recoverable(aErr); ok { lastErr = ve; continue } // validation → let model self-correct
    return nil, aErr // non-recoverable (DB, etc.) → surface
}
return nil, fmt.Errorf("draft invoice: model could not produce a valid invoice after %d attempts: %s", maxRetries+1, lastErr)
```

`recoverable` returns the message for NDIS validation failures, missing-coverage gaps, and bad-code errors (the things a re-prompt can fix); everything else surfaces immediately.

### The Smarts (initial curated set)

1. **Draft invoice from shifts** (rebuild — fixes the bug)
   - gather: participant id + `[from,to]`; load unbilled recorded shifts; attach catalogue candidates per shift (via `CatalogueSearcher`).
   - propose: `create_invoice` schema → `createInvoiceInput`.
   - apply: existing deterministic path — quantity guard → `verifyShiftsCovered` → `CreateWithCatalogPricing` → `billCoveredShifts` (link shifts → drafted). No checkpoint record.
   - result: the created draft `*invoice.Invoice` → `201` JSON. Frontend navigates into it (already does).

2. **Extract shifts from text** (keep — already a Smart)
   - Move `extract.go` + `ImportShifts` behavior under the `Smarts` service unchanged in behavior. Keep endpoint `POST /api/shifts/import`.

**Registry-ready, not built now (out of scope, documented):** *Suggest NDIS code* (text → `{code,name,confidence}`, subtle field autofill) and *Invoice from note* (note → invoice via the non-shifts deterministic path). Each is one schema + one method + one button when wanted.

## Data model

Drop the agent conversational tables. Migration `00002_agent.sql` created (review B3: names are **singular**) `agent_conversation`, `agent_message`, `agent_checkpoint`, `agent_step`, `agent_checkpoint_change`, and `agent_token_usage`. Add a new migration that `DROP TABLE`s all six (token accounting is dropped — see resolved Q1). Regenerate `internal/db/gen` after removing the now-unused queries from `internal/db/queries/agent*.sql`.

FK topology (review S3): all agent FKs are **internal** to the `agent_*` cluster (`agent_message → agent_conversation`, `agent_checkpoint → agent_message`, `agent_step → agent_message`/`agent_checkpoint`, `agent_checkpoint_change → agent_checkpoint`); no non-agent table references them (`shifts.invoice_id → invoices`, not agent). With `foreign_keys=ON`, drop **children before parents** within one migration: `agent_checkpoint_change`, `agent_step`, `agent_checkpoint`, `agent_message`, `agent_conversation`, `agent_token_usage`.

(Clean-break project: no production data migration concern; this is fresh schema.)

## HTTP surface

Remove: `/api/agent/conversations` (×2), `/conversations/{id}/messages` (×2), `/conversations/{id}/stream`, `/agent/steps/{id}/decision`, `/agent/checkpoints/{id}/revert`.
Keep: `POST /api/participants/{id}/draft-invoice` (now Smart-backed), `POST /api/shifts/import` (unchanged behavior).
`/api/events` (the global SSE hub for entity changes) is **separate** from the agent chat stream and stays.

Errors: both Smart handlers return `httpx.WriteError` with the real (or friendly) message — see Open question on which. No raw model/tool strings ever reach the body.

## Frontend

- Delete `web/src/lib/api/agent.ts` + its test.
- `InvoiceSuggestions.svelte`: unchanged behavior — button → `draftInvoice` → navigate to the draft. (Already the right UX; it just stops erroring.)
- Quick-add import on `/` stays; it's already the extract Smart.
- No new components required for the initial set. Future Smarts add a button + a tiny api wrapper each.

## Composition (`internal/app`)

Replace the agent wiring block (registry, checkpoint, budget, restore, `NewAgent`, `NewAgentHandler`) with:

```go
smarts := agent.NewSmarts(agent.Config{...}, llmClient, invoiceSvc, shiftSvc, supportCatalogSvc)
smartsHandler := agent.NewSmartsHandler(smarts, enabled)
```

Routes wire `smartsHandler.DraftInvoiceFromShifts` and `smartsHandler.ImportShifts`.

**Sweep rewiring (review S4 — required for compilation, not optional cleanup):** `runSweepOnce`/`runSweeper` (`sweep.go:26,57`) take `ag *agent.Agent` as a threaded parameter and call `ag.SweepExpired`. The `agent.Agent` type is deleted, so both signatures must drop the `ag` param, the `if ag != nil { ag.SweepExpired }` block (`sweep.go:48-52`) is removed, and the `app.go` call site stops passing `agentSvc`. The invoice/recurring sweeps stay.

**Handler re-home (review S5 — a rewrite, not a field trim):** the kept `ImportShifts` currently hangs off `*AgentHandler`, which embeds deleted types (`agent *Agent`, `store *Store`, `events *Events`, `budget *Budget`). `NewSmartsHandler` re-homes `ImportShifts` + its helpers (`existingShiftKeys`, `shiftDedupKey`, `guard`, `detach`) onto the new struct, which holds only the `Smarts` service + the `enabled` flag. The `WithShiftImport`/`WithRestore`/`WithBudget` builders and `InvoiceRestoreFunc` wiring (`app.go:181`) are deleted.

## Error handling

- `propose` failures (transport/decode/no-tool-call) → non-recoverable → surfaced as a `502`-class error with a friendly message; real error logged via `slog`.
- Deterministic validation failures → recoverable → drive the bounded retry; after exhaustion, one friendly error + logged detail.
- The package name stays `agent` (gut in place) to minimize import churn, though its contents are no longer agentic. (Renaming to `smarts` is a follow-up if desired.)

## Testing

- Keep/adapt the deterministic invoice tests (`tools_invoice_*_test.go`) — point them at `applyDraftInvoice` instead of the `Tool.Handler`.
- New test: errored proposal → bounded retry → friendly typed error (the regression the original bug needed). Use `llm.Fake` to script: attempt 1 returns a bad code, attempt 2 returns a valid one → invoice created.
- New test: `propose` decodes a forced tool_use into the typed struct.
- Keep `extract.go` tests.
- Gate: `go test ./... -race`, `go vet`, `gofmt -l`, `CGO_ENABLED=0 go build .`, `cd web && npm run check && npm run build`.

## Rollout

Single branch (continue `feat/shifts-lifecycle` or a fresh `feat/ai-smarts`). Order: (1) build the `Smarts` service + `propose` + rebuilt draft Smart with tests passing against the deterministic path; (2) rewire `internal/app` + routes; (3) delete harness files + dead endpoints + frontend `agent.ts`; (4) drop tables migration + sqlc regen; (5) full gate. Each step compiles.

## Out of scope

- Building *Suggest code* / *Invoice from note* (documented, deferred).
- Renaming the `agent` package to `smarts`.
- Any chat/conversational capability — explicitly removed, not deferred.

## Resolved decisions

1. **Token accounting — DROP.** Remove `agent_token_usage` + its queries + the `Budget` path entirely. Keeping it would mean retaining a query + a thin caller after `Budget`/`store.go` are deleted; lean-drop is lower risk (review N3). Revisit if cost tracking is needed later.
2. **Error UX — friendly + logged.** Both Smart handlers return a friendly message ("couldn't produce a valid invoice from these shifts" / import equivalent); the real validation/transport detail goes to `slog`. No raw model/tool strings reach the body.
3. **Package name — gut `internal/agent` in place.** Keep the package name to minimize import churn; its contents are no longer agentic. Renaming to `internal/smarts` is a deferred follow-up.

## Notes carried from spec review (2026-06-19)

- `llm.Fake` (`fake.go`) queues `Response`s FIFO with `SetResponses` — the retry regression test (attempt-1 bad code → attempt-2 valid) is directly scriptable; no test-harness gap (N1).
- `requestMaxTokens` and `wrapUntrusted` are relocated to a kept file before their home files are deleted (B1/B2).
- Checkpoint removal verified safe: only consumers are the deleted `Revert` handler and `InvoiceRestoreFunc`/`InvoiceAccessor`; audit is the separate `audit.WithTx` path inside the invoice service, untouched (S2).
