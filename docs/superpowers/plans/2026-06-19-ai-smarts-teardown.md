# AI Smarts Teardown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the conversational AI agent harness with two button-triggered "Smarts" (gather → propose-via-forced-tool → deterministic apply), fixing the `draft-invoice` JSON error structurally.

**Architecture:** Each Smart is a plain method on a `Smarts` service: one forced single-tool `llm.CreateMessage` produces a schema-shaped proposal, deterministic Go validates and applies it (bounded retry ×2 on validation failure), and a plain typed result is returned through the normal error path. No conversation, approval gate, checkpoint, or step persistence. The deterministic invoice-build logic is lifted unchanged from the old `create_invoice` tool; the conversational harness is deleted.

**Tech Stack:** Go 1.26, chi, sqlc + goose (SQLite/modernc), `internal/agent/llm` (Anthropic adapter + `llm.Fake`), SvelteKit SPA.

**Spec:** `docs/superpowers/specs/2026-06-19-ai-smarts-teardown-design.md`

**Strategy:** Build the new Smart path additively (Tasks 1–5) so each task compiles with the old harness still present, cut the routes over (Task 6), then delete the harness wholesale (Task 7) and the tables (Task 8). The package stays `internal/agent` (gut in place). All work on branch `feat/shifts-lifecycle`.

---

## File Structure

**New files (in `internal/agent/`):**
- `smarts.go` — `Smarts` service (struct + `NewSmarts`), and the relocated package helpers `requestMaxTokens` (const) and `wrapUntrusted` (func).
- `propose.go` — the generic `propose[T]` forced-tool helper.
- `smart_draft_invoice.go` — `DraftInvoiceFromShifts` Smart: gather + retry loop + `applyDraftInvoice`, plus the deterministic invoice helpers **moved** here from `tools_invoice.go`.
- `smarts_handler.go` — `SmartsHandler` (struct + `NewSmarts­Handler`), the `DraftInvoiceFromShifts` + `ImportShifts` HTTP handlers, and the moved handler helpers (`guard`, `detach`, `existingShiftKeys`, `shiftDedupKey`).

**Deleted at Task 7:** `agent.go`, `permission.go`, `plan.go`, `checkpoint.go`, `budget.go`, `store.go`, `stream.go`, `sweep.go`, `tools.go`, `context.go`, `prompt.go`, `agent_handler.go`, and the `Tool`-wrapper bodies in `tools_invoice.go`/`tools_shifts.go` (the files shrink to nothing → deleted). `deps.go` trimmed.

**Migration (Task 8):** new `internal/db/migrations/00007_drop_agent_chat.sql`; remove `internal/db/queries/agent*.sql`; `sqlc generate`.

**Frontend (Task 9):** delete `web/src/lib/api/agent.ts` + `agent.test.ts`.

---

## Task 1: Relocate shared helpers into a new `smarts.go`

Move the two symbols that kept code (`extract.go`, `tools_shifts.go`) depends on OUT of files slated for deletion, so later deletion is clean. Create the `Smarts` service skeleton.

**Files:**
- Create: `internal/agent/smarts.go`
- Modify: `internal/agent/context.go` (remove `requestMaxTokens` const), `internal/agent/prompt.go` (remove `wrapUntrusted` func, keep `SystemPrompt`)

- [ ] **Step 1: Create `smarts.go`** with the service + relocated helpers.

```go
package agent

import (
	"strings"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// requestMaxTokens is the per-call output ceiling for a Smart's single model
// turn. Relocated from the deleted context.go; also used by extract.go.
const requestMaxTokens = 64000

// Smarts is the AI capability surface: a small curated set of one-shot actions,
// each gather → propose → apply. It depends only on the slice services it needs
// and the model client; no conversation/step/checkpoint state.
type Smarts struct {
	cfg     Config
	client  llm.Client
	invoice InvoiceCreator
	shifts  ShiftWorker
	catalog CatalogueSearcher
}

// NewSmarts constructs the Smarts service. A nil dependency is a programmer error.
func NewSmarts(cfg Config, client llm.Client, inv InvoiceCreator, shifts ShiftWorker, catalog CatalogueSearcher) *Smarts {
	if client == nil || inv == nil || shifts == nil || catalog == nil {
		panic("agent: NewSmarts requires non-nil client, invoice, shifts, catalog")
	}
	return &Smarts{cfg: cfg, client: client, invoice: inv, shifts: shifts, catalog: catalog}
}

// wrapUntrusted fences arbitrary record text so the model treats it as data
// rather than instructions. Relocated from the deleted prompt.go; used by
// extract.go and the draft-invoice gather.
func wrapUntrusted(label, body string) string {
	sanitised := strings.ReplaceAll(body, "</untrusted-content", "&lt;/untrusted-content")
	return "<untrusted-content source=\"" + label + "\">\n" + sanitised + "\n</untrusted-content>"
}
```

- [ ] **Step 2: Remove `requestMaxTokens` from `context.go`.** Delete the const declaration (around `internal/agent/context.go:11`). Leave `buildRequest` for now (deleted in Task 7).

- [ ] **Step 3: Remove `wrapUntrusted` from `prompt.go`.** Delete the func (around `internal/agent/prompt.go:42`) and the now-unused `import "strings"` if nothing else uses it. Keep `SystemPrompt`.

- [ ] **Step 4: Build.**

Run: `CGO_ENABLED=0 go build ./...`
Expected: PASS (helpers now live in `smarts.go`; same package, all references resolve).

- [ ] **Step 5: Commit.**

```bash
git add internal/agent/smarts.go internal/agent/context.go internal/agent/prompt.go
git commit -m "refactor(agent): add Smarts service skeleton; relocate shared helpers"
```

---

## Task 2: The `propose[T]` forced-tool helper (TDD)

**Files:**
- Create: `internal/agent/propose.go`
- Test: `internal/agent/propose_test.go`

- [ ] **Step 1: Write the failing test.** Uses `llm.Fake` to return one forced `tool_use` block; asserts decode into a typed struct.

```go
package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

type proposeProbe struct {
	Code     string `json:"code"`
	Quantity int    `json:"quantity"`
}

func TestProposeDecodesForcedToolUse(t *testing.T) {
	fake := &llm.Fake{}
	// SetResponses is variadic by VALUE: func (f *Fake) SetResponses(rs ...Response).
	fake.SetResponses(llm.Response{
		StopReason: llm.StopToolUse,
		Content: []llm.Block{{
			Type:     llm.BlockToolUse,
			ToolName: "emit",
			Input:    json.RawMessage(`{"code":"01_011_0107_1_1","quantity":3}`),
		}},
	})
	got, err := propose[proposeProbe](context.Background(), fake, Config{Model: "claude-x"},
		"system", "user", "emit", json.RawMessage(`{"type":"object"}`))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if got.Code != "01_011_0107_1_1" || got.Quantity != 3 {
		t.Fatalf("decoded = %+v", got)
	}
}

func TestProposeErrorsWhenNoToolCall(t *testing.T) {
	fake := &llm.Fake{}
	fake.SetResponses(llm.Response{
		StopReason: llm.StopEndTurn,
		Content:    []llm.Block{{Type: llm.BlockText, Text: "I refuse"}},
	})
	_, err := propose[proposeProbe](context.Background(), fake, Config{Model: "claude-x"},
		"system", "user", "emit", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when model emits no tool call")
	}
}
```

> Confirmed API (`internal/agent/llm/fake.go:35`): `func (f *Fake) SetResponses(rs ...Response)` — variadic **values**, not a pointer slice. Pass `llm.Response{...}, llm.Response{...}`. Do NOT change the fake.

- [ ] **Step 2: Run test to verify it fails.**

Run: `go test ./internal/agent/ -run TestPropose -v`
Expected: FAIL — `undefined: propose`.

- [ ] **Step 3: Write `propose.go`.**

```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// propose forces the model to emit exactly one tool_use whose input matches the
// schema, and decodes it into T. One call — no conversation, no history. The
// model's only job is to fill the schema; deterministic Go owns everything after.
func propose[T any](ctx context.Context, c llm.Client, cfg Config,
	system, userContent, toolName string, schema json.RawMessage) (T, error) {

	var zero T
	req := llm.Request{
		System:     system,
		Tools:      []llm.ToolDef{{Name: toolName, InputSchema: schema}},
		ToolChoice: llm.ToolChoice{ForceTool: toolName},
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: []llm.Block{{Type: llm.BlockText, Text: userContent}},
		}},
		MaxTokens: requestMaxTokens,
		Model:     cfg.Model,
		Effort:    cfg.EffortFor(),
	}
	resp, err := c.CreateMessage(ctx, req)
	if err != nil {
		return zero, fmt.Errorf("propose %s: %w", toolName, err)
	}
	if resp == nil {
		return zero, fmt.Errorf("propose %s: nil response", toolName)
	}
	for i := range resp.Content { // bounded by len(Content)
		b := resp.Content[i]
		if b.Type == llm.BlockToolUse && b.ToolName == toolName {
			var out T
			if e := json.Unmarshal(b.Input, &out); e != nil {
				return zero, fmt.Errorf("propose %s: decode input: %w", toolName, e)
			}
			return out, nil
		}
	}
	return zero, fmt.Errorf("propose %s: model emitted no %s call", toolName, toolName)
}
```

- [ ] **Step 4: Run test to verify it passes.**

Run: `go test ./internal/agent/ -run TestPropose -v`
Expected: PASS (both).

- [ ] **Step 5: Commit.**

```bash
git add internal/agent/propose.go internal/agent/propose_test.go
git commit -m "feat(agent): add propose[T] forced-tool structured-output helper"
```

---

## Task 3: Move the deterministic invoice build into `applyDraftInvoice` (TDD)

Move the pure invoice-build logic out of `tools_invoice.go` (whose `Tool` wrappers die in Task 7) into `smart_draft_invoice.go`, and expose it as `applyDraftInvoice` — the same logic minus the checkpoint recording.

**Files:**
- Create: `internal/agent/smart_draft_invoice.go`
- Modify: `internal/agent/tools_invoice.go` (remove the moved symbols)
- Test: adapt `internal/agent/tools_invoice_shifts_test.go` (already exercises this logic)

- [ ] **Step 1: Move these symbols verbatim** from `tools_invoice.go` into a new `smart_draft_invoice.go` (cut/paste, keep the package clause): `createInvoiceInput` struct, `createInvoiceSchema` const, `verifyShiftsCovered`, `billCoveredShifts`, `coverageRange`, `hasCodedLine`, `codedDateRange`, `round2c` (any pure helper the first three call). Leave the `New*Tool` wrapper funcs in `tools_invoice.go` (they still compile — same package — until Task 7).

- [ ] **Step 2: Add `applyDraftInvoice`** to `smart_draft_invoice.go`. This is the body of `newCreateInvoiceToolShifts`'s handler with the `checkpointFrom`/`cp.Record` block removed and `Result` replaced by the typed return.

```go
// applyDraftInvoice is the deterministic half of the draft-invoice Smart: it
// validates the model's proposal, verifies it covers every recorded shift in the
// window, prices coded lines from the catalogue, persists the invoice, and links
// the covered shifts (status → drafted). No checkpoint recording — the draft is
// itself the reviewable artifact. Returns a billing.ValidationError-wrapped error
// (recoverable) for proposals the model can fix on retry.
func (s *Smarts) applyDraftInvoice(ctx context.Context, in createInvoiceInput) (*invoice.Invoice, error) {
	if in.ParticipantID <= 0 {
		return nil, fmt.Errorf("draft invoice: participantId must be a positive integer")
	}
	if len(in.Items) == 0 {
		return nil, fmt.Errorf("draft invoice: at least one line item is required")
	}
	for i := range in.Items { // bounded by len(in.Items)
		it := in.Items[i]
		if it.Code != "" && it.Quantity <= 0 {
			return nil, fmt.Errorf("draft invoice: line %d (code %q) needs a quantity greater than 0", i, it.Code)
		}
	}

	coverFrom, coverTo := in.From, in.To
	if err := verifyShiftsCovered(ctx, s.shifts, in.ParticipantID, in.Items, coverFrom, coverTo); err != nil {
		return nil, err // already prefixed; recoverable (coverage gap)
	}

	header := invoice.InvoiceInput{
		ParticipantID: in.ParticipantID,
		PlanManagerID: in.PlanManagerID,
		IssueDate:     in.IssueDate,
		DueDate:       in.DueDate,
		Notes:         in.Notes,
	}
	created, err := s.invoice.CreateWithCatalogPricing(ctx, header, in.Items)
	if err != nil {
		if ve, ok := billing.AsValidationError(err); ok {
			return nil, fmt.Errorf("draft invoice: invoice failed NDIS validation: %s", ve.Error())
		}
		return nil, fmt.Errorf("draft invoice: %w", err)
	}

	billCoveredShifts(ctx, s.shifts, in.ParticipantID, created.ID, coverFrom, coverTo, in.Items)
	return created, nil
}
```

Add the imports it needs (`context`, `fmt`, `github.com/dknathalage/tallyo/internal/billing`, `.../internal/invoice`).

- [ ] **Step 3: Adapt the existing test.** In `tools_invoice_shifts_test.go`, the `shiftsCreateFixture` helper currently **returns a `Tool`** (~line 58) and tests call `tool.Handler(...)`. Rewrite the fixture to return a `*Smarts` (built over the real test services it already constructs — `Smarts{invoice: inv, shiftSvc: shiftSvc, shifts: shiftSvc, catalog: cat}`), and change each call site to `s.applyDraftInvoice(ctx, in)` where `in` is the same `createInvoiceInput` the test marshals. Assert on the returned `*invoice.Invoice` (not `res.JSON.(*invoice.Invoice)`). The fixture must no longer reference `Tool`/`.Handler` so the file survives Task 7.

- [ ] **Step 4: Run tests.**

Run: `go test ./internal/agent/ -run Invoice -v`
Expected: PASS (coverage-verify, catalogue-pricing, quantity-guard behaviors preserved).

- [ ] **Step 5: Build + commit.**

Run: `CGO_ENABLED=0 go build ./...`
```bash
git add internal/agent/smart_draft_invoice.go internal/agent/tools_invoice.go internal/agent/tools_invoice_shifts_test.go internal/agent/tools_invoice_create_test.go
git commit -m "refactor(agent): lift deterministic invoice build into applyDraftInvoice"
```

> If `tools_invoice_create_test.go` (the non-shifts path) references the moved `createInvoiceInput`/`createInvoiceSchema`, it still compiles (same package). Adapt it only if it broke.

---

## Task 4: The draft-invoice Smart — gather + retry loop (TDD)

**Files:**
- Modify: `internal/agent/smart_draft_invoice.go` (add gather + `DraftInvoiceFromShifts` method + system prompt + `maxDraftRetries`)
- Modify: `internal/agent/tools_shifts.go` (extract `shiftCandidates` resolution so gather can reuse it) — or copy the minimal candidate-resolution into a new gather helper if `shiftCandidates` is too entangled with `shiftView`
- Test: `internal/agent/smart_draft_invoice_test.go`

- [ ] **Step 1: Write the failing test** — bad code on attempt 1, valid on attempt 2 → invoice created; and exhaustion → friendly error. Use the test fixtures from `tools_invoice_shifts_test.go` (a participant with recorded shifts + a seeded catalogue) and an `llm.Fake` scripted with two `create_invoice` tool_use responses.

```go
func TestDraftInvoiceRetriesThenSucceeds(t *testing.T) {
	s, ctx, pid, from, to := newDraftSmartFixture(t) // builds Smarts over real test services + seeded shifts/catalogue
	badInput := /* createInvoiceInput JSON: a line with a wrong/uncoded item → fails verifyShiftsCovered or NDIS validation */
	goodInput := /* correct createInvoiceInput covering the shifts */
	fake := s.client.(*llm.Fake)
	fake.SetResponses( // variadic VALUES (...Response)
		toolUse("create_invoice", badInput),
		toolUse("create_invoice", goodInput),
	)
	inv, err := s.DraftInvoiceFromShifts(ctx, pid, from, to)
	if err != nil {
		t.Fatalf("DraftInvoiceFromShifts: %v", err)
	}
	if inv == nil || inv.ID == 0 {
		t.Fatalf("no invoice created")
	}
}
```

> `toolUse(name, raw)` is a tiny test helper returning a **value** `llm.Response{StopReason: llm.StopToolUse, Content: []llm.Block{{Type: llm.BlockToolUse, ToolName: name, Input: raw}}}` (not a pointer — `SetResponses` takes `...Response`). Add it to the test file.

- [ ] **Step 2: Run test to verify it fails.**

Run: `go test ./internal/agent/ -run TestDraftInvoice -v`
Expected: FAIL — `undefined: (*Smarts).DraftInvoiceFromShifts`.

- [ ] **Step 3: Implement gather + method.** Add to `smart_draft_invoice.go`:

```go
const maxDraftRetries = 2

// draftInvoiceSystem instructs the model to map recorded shifts to catalogue
// codes and emit ONE create_invoice call. (Lifted from the old SystemPrompt's
// "Drafting Invoices From Shifts" section, trimmed to the single-shot job.)
const draftInvoiceSystem = `You convert a participant's recorded shifts into an NDIS invoice. ...`

// DraftInvoiceFromShifts is the Smart: gather the participant's unbilled shifts
// (+ catalogue candidates) for [from,to], ask the model to map them to a
// create_invoice proposal, and apply it deterministically. On a recoverable
// validation failure it feeds the error back and re-proposes, bounded by
// maxDraftRetries — NOT a conversation.
func (s *Smarts) DraftInvoiceFromShifts(ctx context.Context, participantID int64, from, to string) (*invoice.Invoice, error) {
	if participantID <= 0 {
		return nil, fmt.Errorf("draft invoice: invalid participant id")
	}
	if from == "" || to == "" {
		return nil, fmt.Errorf("draft invoice: from and to are required")
	}
	base, err := s.gatherShiftContext(ctx, participantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("draft invoice: gather: %w", err)
	}

	var lastErr string
	for attempt := 0; attempt <= maxDraftRetries; attempt++ { // bounded
		content := base
		if lastErr != "" {
			content = base + "\n\nYour previous attempt failed:\n" + lastErr + "\nFix it and emit create_invoice again."
		}
		proposal, pErr := propose[createInvoiceInput](ctx, s.client, s.cfg,
			draftInvoiceSystem, content, "create_invoice", json.RawMessage(createInvoiceSchema))
		if pErr != nil {
			return nil, pErr // transport/decode/no-call: not retryable here
		}
		proposal.ParticipantID = participantID // trust the URL, not the model
		proposal.From, proposal.To = from, to
		inv, aErr := s.applyDraftInvoice(ctx, proposal)
		if aErr == nil {
			return inv, nil
		}
		if recoverableDraftErr(aErr) {
			lastErr = aErr.Error()
			continue
		}
		return nil, aErr // non-recoverable (DB, etc.)
	}
	return nil, fmt.Errorf("draft invoice: could not produce a valid invoice after %d attempts: %s", maxDraftRetries+1, lastErr)
}

// recoverableDraftErr reports whether the model can plausibly fix err on retry
// (NDIS validation failure, coverage gap, bad code/quantity). DB/transport
// errors are not recoverable.
func recoverableDraftErr(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "NDIS validation") ||
		strings.Contains(msg, "does not cover every recorded shift") ||
		strings.Contains(msg, "needs a quantity greater than 0")
}
```

- [ ] **Step 4: Implement `gatherShiftContext`** — load the participant's unbilled recorded shifts for `[from,to]` via `s.shifts.ListParticipant`, attach per-shift catalogue candidates, and render a compact, `wrapUntrusted`-fenced text block listing each shift's date/hours/km + candidate codes. Bounded by the number of shifts. (Mirror what the old `list_participant_shifts` tool fed the model.)

  > `shiftCandidates` (`tools_shifts.go:129`) is **already a free function** `func shiftCandidates(ctx, cat CatalogueSearcher, sh *shift.Shift) []candidateView` with zero `shiftView`/`Tool` dependency — only its call site sits in the tool handler. Keep `shiftCandidates` + `candidateView` verbatim and call them directly from `gatherShiftContext`; delete only the surrounding `Tool` wrapper (Task 7). No extraction needed.

- [ ] **Step 5: Run test to verify it passes.**

Run: `go test ./internal/agent/ -run TestDraftInvoice -v`
Expected: PASS.

- [ ] **Step 6: Build + commit.**

```bash
git add internal/agent/smart_draft_invoice.go internal/agent/smart_draft_invoice_test.go internal/agent/tools_shifts.go
git commit -m "feat(agent): draft-invoice Smart (gather → propose → apply, bounded retry)"
```

---

## Task 5: `SmartsHandler` + `Smarts.ImportShifts` — HTTP surface for both Smarts

**This task is PURELY ADDITIVE plus relocating shared free symbols.** It must NOT remove the old `AgentHandler.DraftInvoiceFromShifts`/`AgentHandler.ImportShifts` methods — `server.go` still routes to them until Task 6, so removing them would break the build. The old `AgentHandler` and its chat handlers stay intact and are deleted wholesale in Task 7.

Key constraint: `detach` and `shiftDedupKey` are **free functions** and `draftInvoiceRequest`/`importShiftsRequest` are **types**, all currently in `agent_handler.go`. We RELOCATE them (cut → paste) into the new smart files so they survive Task 7's deletion of `agent_handler.go`. Because everything is one package, the old chat handlers and old methods still resolve them after the move — no duplication, no breakage. `guard`/`existingShiftKeys` are `AgentHandler` *methods*; do NOT move those — give the new types their own copies (different receiver = no conflict), and the old ones die with `agent_handler.go` in Task 7.

**Files:**
- Modify: `internal/agent/deps.go` (extend `ShiftWorker` with `Create`)
- Create: `internal/agent/smart_import_shifts.go` (the `Smarts.ImportShifts` method + relocated `shiftDedupKey` + a `Smarts.existingShiftKeys`)
- Create: `internal/agent/smarts_handler.go` (`SmartsHandler` + both HTTP handlers + a `SmartsHandler.guard` + relocated `detach` + relocated request structs)
- Modify: `internal/agent/agent_handler.go` (ONLY: cut out the relocated free funcs `detach`, `shiftDedupKey` and the relocated structs `draftInvoiceRequest`, `importShiftsRequest`; leave everything else — including the old `DraftInvoiceFromShifts`/`ImportShifts`/`guard`/`existingShiftKeys` — untouched)

- [ ] **Step 1: Extend `ShiftWorker` in `deps.go`.** Add a creator interface and embed it so `Smarts.shifts` can create shifts (the import path needs `Create`). `*shift.Service` already satisfies it; no call-site changes elsewhere.

```go
// ShiftCreator is satisfied by *shift.Service; the import-shifts Smart creates
// recorded shifts from extracted drafts.
type ShiftCreator interface {
	Create(ctx context.Context, in shift.ShiftInput) (*shift.Shift, error)
}

// ShiftWorker composes the shift reads/writes the Smarts use.
type ShiftWorker interface {
	ShiftLister
	ShiftDrafter
	ShiftCreator
}
```

Verify `shift.Service.Create` has exactly this signature first; match it.

- [ ] **Step 2: `Smarts.ImportShifts` in `smart_import_shifts.go`.** Port the body of the old `AgentHandler.ImportShifts` as a service method:
  `func (s *Smarts) ImportShifts(ctx context.Context, participantID int64, text string) ([]*shift.Shift, error)` — call `ExtractShifts(ctx, s.client, s.cfg.Model, s.cfg.EffortFor(), text)`, dedup against existing recorded shifts (port `existingShiftKeys` as `(s *Smarts) existingShiftKeys(...)` using `s.shifts.ListParticipant`), create survivors via `s.shifts.Create(...)`. **Relocate the free func `shiftDedupKey`** here (cut from `agent_handler.go`). Return the created shifts or a wrapped error (no HTTP concerns here).

- [ ] **Step 3: `smarts_handler.go`** — the struct + constructor:

```go
type SmartsHandler struct {
	smarts  *Smarts
	enabled bool
}

func NewSmartsHandler(s *Smarts, enabled bool) *SmartsHandler {
	if enabled && s == nil {
		panic("NewSmartsHandler: enabled handler requires a non-nil Smarts")
	}
	return &SmartsHandler{smarts: s, enabled: enabled}
}
```
Add `(h *SmartsHandler) guard(w, r) (tenantID, userID int64, ok bool)` — a copy of `AgentHandler.guard`'s logic (checks `h.enabled` → 503, pulls tenant/user from `reqctx`). **Relocate the free func `detach`** here (cut from `agent_handler.go`).
- `(h *SmartsHandler) DraftInvoiceFromShifts(w, r)`: `guard` → `httpx.ParseID(r)` → decode `draftInvoiceRequest{from,to}` → validate non-empty → `ctx,cancel := context.WithTimeout(detach(tid,uid), 5*time.Minute)` → `inv, err := h.smarts.DraftInvoiceFromShifts(ctx, pid, from, to)` → on err `slog.Error(...)` + `httpx.WriteError(w, http.StatusBadGateway, "couldn't produce a valid invoice from these shifts")`; on success `httpx.WriteJSON(w, http.StatusCreated, inv)`.
- `(h *SmartsHandler) ImportShifts(w, r)`: `guard` → decode `importShiftsRequest{participantId,text}` → validate → `ctx,cancel := context.WithTimeout(detach(tid,uid), 2*time.Minute)` → `created, err := h.smarts.ImportShifts(ctx, pid, text)` → on err `slog.Error(...)` + `httpx.WriteError(w, http.StatusBadGateway, "could not extract shifts from the timesheet")`; on success `httpx.WriteJSON(w, http.StatusCreated, created)`.
- **Relocate the structs** `draftInvoiceRequest`, `importShiftsRequest` here (cut from `agent_handler.go`).

- [ ] **Step 4: Trim `agent_handler.go`** — cut ONLY the four relocated symbols (`detach`, `shiftDedupKey`, `draftInvoiceRequest`, `importShiftsRequest`). Do NOT remove any method. The old `DraftInvoiceFromShifts`/`ImportShifts`/`guard`/`existingShiftKeys`/chat handlers still reference `detach`/`shiftDedupKey`/the structs — now resolved in-package from the new files.

- [ ] **Step 5: Build + the agent tests.**

Run: `CGO_ENABLED=0 go build ./...` (MUST pass — whole module, incl. `server.go` still using old `AgentHandler`)
Run: `go test ./internal/agent/ -run 'Invoice|Draft|Import|Extract' -v` (MUST pass)
Then `gofmt -w` the touched files.

> If the build fails with a duplicate declaration, you copied instead of moved a free func/struct — delete the original. If it fails with "undefined", you removed something still referenced — restore it.

- [ ] **Step 6: Commit.**

```bash
git add internal/agent/deps.go internal/agent/smart_import_shifts.go internal/agent/smarts_handler.go internal/agent/agent_handler.go
git commit -m "feat(agent): SmartsHandler + Smarts.ImportShifts (additive; relocate shared helpers)"
```

---

## Task 6: Wire `internal/app` to Smarts; cut routes over; fix sweep

**Files:**
- Modify: `internal/app/app.go` (replace agent wiring block ~`:164-185`), `internal/app/server.go` (`Agent` field → `Smarts`; routes `:213-222`), `internal/app/sweep.go` (drop the `ag *agent.Agent` param)

- [ ] **Step 1: Replace the agent wiring in `app.go`** (`~:164-186`). Remove **all** of: `var agentSvc *agent.Agent` and `var agentHandler *agent.AgentHandler`; `agent.NewStore`, `agent.NewEvents`, `agent.NewRegistry`, `agent.NewCheckpoint`, `agent.NewBudgetWallClock`; the `agentReg.Register(...)` calls; `agent.NewAgent(...).WithBudget(...).WithRestore(agent.InvoiceRestoreFunc(...))`; and `agent.NewAgentHandler(...).WithShiftImport(...)`. The `var agentSvc` is also threaded into the sweep call site (`~:237,239`) — drop that argument there too (see Step 3). Replace the block with:

```go
var smartsHandler *agent.SmartsHandler
if agentCfg.APIKey != "" {
	smarts := agent.NewSmarts(agentCfg, llmClient, invoiceSvc, shiftSvc, supportCatalogSvc)
	smartsHandler = agent.NewSmartsHandler(smarts, true)
} else {
	smartsHandler = agent.NewSmartsHandler(nil, false)
}
```

(Confirm `shiftSvc` satisfies `ShiftWorker` + the `ShiftCreator`/concrete decision from Task 5; `supportCatalogSvc` satisfies `CatalogueSearcher`; `invoiceSvc` satisfies `InvoiceCreator`.)

- [ ] **Step 2: Update `server.go` `Deps`.** Rename `Agent *agent.AgentHandler` → `Smarts *agent.SmartsHandler`. In the route block, **delete** the chat routes (`/agent/conversations` ×2, `/agent/conversations/{id}/messages` ×2, `/agent/conversations/{id}/stream`, `/agent/steps/{id}/decision`, `/agent/checkpoints/{id}/revert`) and keep only:

```go
if deps.Smarts != nil {
	pr.Post("/participants/{id}/draft-invoice", deps.Smarts.DraftInvoiceFromShifts)
	pr.Post("/shifts/import", deps.Smarts.ImportShifts)
}
```

Update the `deps.Agent != nil` references in the big `||` guard at `server.go:159` and the assembly to `deps.Smarts`.

- [ ] **Step 3: Fix the sweep.** In `sweep.go`, drop the `ag *agent.Agent` parameter from `runSweepOnce`/`runSweeper`, delete the `if ag != nil { ag.SweepExpired(...) }` block, and remove the `agentSvc` argument at the `app.go` call site. Keep the invoice + recurring sweeps.

- [ ] **Step 4: Build.**

Run: `CGO_ENABLED=0 go build ./...`
Expected: FAIL ONLY in the to-be-deleted harness files (if any still reference removed wiring). The `app`/`server`/`sweep` packages should compile. If `app.go` still imports unused agent symbols, that's expected until Task 7 — proceed.

> If the build fails outside `internal/agent`, fix it now. Failures inside `internal/agent` (e.g. `agent_handler.go` chat handlers referencing a now-renamed field) are resolved by Task 7's deletion.

- [ ] **Step 5: Commit.**

```bash
git add internal/app/app.go internal/app/server.go internal/app/sweep.go
git commit -m "refactor(app): route AI through SmartsHandler; drop agent sweep + chat routes"
```

---

## Task 7: Delete the conversational harness

**Files (delete):** `internal/agent/agent.go`, `permission.go`, `plan.go`, `checkpoint.go`, `budget.go`, `store.go`, `stream.go`, `sweep.go`, `tools.go`, `context.go`, `prompt.go`, `agent_handler.go`. Plus their `_test.go` siblings. **Modify:** `tools_invoice.go`, `tools_shifts.go` (delete the `Tool`-wrapper funcs; if a file becomes empty, delete it), `deps.go` (trim).

- [ ] **Step 1: Delete the harness files + every test that references deleted `Tool`/`Registry`/harness machinery.**

First list what's actually present: `ls internal/agent/*_test.go`. Then delete (review B2 — the four `tools*_test.go` files below are **required**: they build `Tool`/`Registry`/`.Handler`, all deleted here):

```bash
cd internal/agent
git rm agent.go agent_test.go permission.go permission_test.go plan.go plan_test.go \
  checkpoint.go checkpoint_test.go budget.go budget_test.go store.go store_test.go \
  stream.go stream_test.go sweep.go sweep_test.go tools.go tools_test.go \
  context.go context_test.go prompt.go prompt_test.go agent_handler.go agent_handler_test.go \
  tools_invoice_create_test.go tools_shifts_test.go
```

> `tools_test.go` (`NewRegistry`/`Tool{}`), `tools_invoice_create_test.go` (`NewCreateInvoiceTool(...).Handler`), `tools_shifts_test.go` (`NewListParticipantShiftsTool*`/`.Handler`) all exercise deleted machinery — delete them. The deterministic invoice behaviors they covered are now under `applyDraftInvoice` (Task 3) and the draft-Smart test (Task 4); `tools_invoice_shifts_test.go` was repointed at `applyDraftInvoice` in Task 3 and stays. `extract_test.go` (if present) stays — it tests the kept Smart.

- [ ] **Step 1b: Delete the `internal/app` tests that exercise the old harness.** `internal/app/agent_test.go` and `internal/app/shift_import_test.go` build the old `agent.AgentHandler`/`agent.NewAgent` directly and will not compile once the types are gone:
```bash
git rm internal/app/agent_test.go internal/app/shift_import_test.go
```
(The kept Smart behaviors are covered by `internal/agent` tests; the import-shift HTTP path loses app-level coverage — acceptable, flagged for the final review.)

- [ ] **Step 2: Strip `Tool` wrappers.** In `tools_invoice.go` delete `NewListInvoicesTool`, `NewCreateInvoiceTool`, `NewCreateInvoiceToolForShifts`, `newCreateInvoiceTool`, `newCreateInvoiceToolShifts`, `NewInvoiceRestoreFunc`/`InvoiceRestoreFunc`. In `tools_shifts.go` delete the `New*Tool` funcs and the `shiftView` plumbing that only served them — but KEEP the candidate-resolution helper the gather reuses (moved/kept in Task 4). If either file is now empty, `git rm` it.

- [ ] **Step 3: Trim `deps.go`.** Remove `InvoiceLister`, `InvoiceAccessor`, and any interface no longer referenced. Keep `InvoiceCreator`, `ShiftLister`, `ShiftDrafter`, `ShiftWorker`, `CatalogueSearcher` (+ `ShiftCreator` if added in Task 5). Remove now-unused imports.

- [ ] **Step 4: Build + vet + the agent tests.**

Run: `CGO_ENABLED=0 go build ./... && go vet ./... && go test ./internal/agent/ -race -v`
Expected: PASS. Fix any remaining dangling references (the compiler names them).

- [ ] **Step 5: Commit.**

```bash
git add -A internal/agent
git commit -m "refactor(agent): delete conversational harness (loop, approval, checkpoint, store, stream)"
```

---

## Task 8: Drop the agent chat tables + regenerate sqlc

**Files:**
- Create: `internal/db/migrations/00007_drop_agent_chat.sql`
- Delete/trim: `internal/db/queries/agent*.sql`
- Regenerate: `internal/db/gen/`

- [ ] **Step 1: Confirm the next migration number.**

Run: `ls internal/db/migrations/`
Expected: highest is `00006_*`; use `00007`.

- [ ] **Step 2: Write the drop migration** (children before parents, per spec S3).

```sql
-- +goose Up
DROP TABLE IF EXISTS agent_checkpoint_change;
DROP TABLE IF EXISTS agent_step;
DROP TABLE IF EXISTS agent_checkpoint;
DROP TABLE IF EXISTS agent_message;
DROP TABLE IF EXISTS agent_conversation;
DROP TABLE IF EXISTS agent_token_usage;

-- +goose Down
-- Clean-break: the agent chat schema is not restored. (No-op down.)
SELECT 1;
```

- [ ] **Step 3: Remove the agent queries.** `git rm internal/db/queries/agent.sql` (and any other `agent*.sql`). These backed `store.go`/`budget.go`, now deleted.

- [ ] **Step 4: Regenerate gen.**

Run: `"$(go env GOPATH)/bin/sqlc" generate`
Expected: `internal/db/gen` loses the `AgentConversation`/`AgentMessage`/`AgentStep`/`AgentCheckpoint`/`AgentCheckpointChange`/`AgentTokenUsage` models + their query methods. No errors.

- [ ] **Step 5: Build + start-up migration smoke.**

Run: `CGO_ENABLED=0 go build ./... && go test ./internal/db/... ./internal/app/... -race`
Expected: PASS — migrations apply cleanly on a fresh DB (drop is idempotent via `IF EXISTS`).

- [ ] **Step 6: Commit.**

```bash
git add internal/db
git commit -m "feat(db): drop agent chat/checkpoint tables; remove agent queries"
```

---

## Task 9: Delete the dead frontend agent client

**Files (delete):** `web/src/lib/api/agent.ts`, `web/src/lib/api/agent.test.ts`

- [ ] **Step 1: Confirm no importers.**

Run: `grep -rn "api/agent" web/src --include=*.ts --include=*.svelte | grep -v "agent.ts\|agent.test"`
Expected: no output (the `draftInvoice`/`importShifts` wrappers live in `shifts.ts`, untouched).

- [ ] **Step 2: Delete.**

```bash
git rm web/src/lib/api/agent.ts web/src/lib/api/agent.test.ts
```

- [ ] **Step 3: Frontend gate.**

Run: `cd web && npm run check`
Expected: 0 errors / 0 warnings.

- [ ] **Step 4: Commit.**

```bash
git add -A web/src
git commit -m "chore(web): remove unused agent chat API client"
```

---

## Task 10: Full gate + manual verification

- [ ] **Step 1: Backend gate.**

Run: `go test ./... -race && go vet ./... && gofmt -l . && CGO_ENABLED=0 go build .`
Expected: tests PASS, vet clean, `gofmt -l` prints nothing, binary builds.

- [ ] **Step 2: Frontend gate.**

Run: `cd web && npm run check && npm run build`
Expected: 0/0, `web/build` emitted.

- [ ] **Step 3: Manual smoke (the original bug).** With an API key configured and a participant who has ≥1 unbilled recorded shift in a range: click **Draft invoice** in `InvoiceSuggestions`. Expected: navigates into a draft invoice (no `Unexpected token` error). With a deliberately un-resolvable shift, expect the friendly error string, and the real reason in server logs.

- [ ] **Step 4: Final commit (if any formatting fixups).**

```bash
git add -A && git commit -m "chore: gofmt + final cleanup for AI Smarts teardown" || true
```

---

## Notes for the implementer

- **Package stays `internal/agent`** despite no longer being agentic (gut in place; rename is a deferred follow-up). Don't rename mid-plan.
- **Do not reintroduce** conversation/step/checkpoint/approval/streaming concepts. The editable draft is the review surface; errors return through the normal `httpx.WriteError` path. (See skill `designing-ai-smarts`.)
- **`llm.Fake`** queues responses FIFO via `SetResponses` — verify the exact API in `internal/agent/llm/fake.go` before writing Task 2/4 tests; adapt tests to the fake, never the reverse.
- **Catalogue candidates** are the one non-obvious lift (Task 4): the resolution logic is inside `tools_shifts.go`'s `shiftCandidates`. Extract the resolver, drop the `shiftView`/`Tool` wrapper.
- If any task's build breaks **outside** `internal/agent`, stop and fix before proceeding; breaks **inside** `internal/agent` are expected until Task 7.
