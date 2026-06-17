# Shifts Lifecycle + AI Invoicing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace per-day "notes" with a first-class **Shift** entity carrying a `scheduled → recorded → drafted → sent → paid` lifecycle, drive invoicing from recorded shifts via the existing AI harness (auto-approved, no chat), and rebuild the SPA around Shifts/Calendar/Participants (chat UI removed).

**Architecture:** Evolve the committed `notes` table/repo/service into `shifts` (clean-break migration — no prod data). Reuse the agent harness (catalogue-authoritative pricing + completeness verify) keyed on shifts; add a text→shifts extraction capability and invoice suggestions. Rebuild the SvelteKit SPA IA; remove the assistant chat UI (keep the agent backend + `api/agent.ts`). Table via `@careswitch/svelte-data-table`.

**Tech Stack:** Go 1.26 (chi, sqlc, goose, modernc SQLite), SvelteKit + Svelte 5 runes + Tailwind 4, anthropic-sdk-go.

**Reference:** spec `docs/superpowers/specs/2026-06-17-shifts-lifecycle-redesign-design.md`; prototype `.superpowers/brainstorm/51943-1781699403/prototype-v6.html`.

---

## File map

**Backend**
- `internal/db/migrations/00004_shifts.sql` (create) — evolve notes→shifts.
- `internal/db/queries/shifts.sql` (create) — sqlc source; regenerate `internal/db/gen`.
- `internal/repository/shift.go` (create; supersedes `note.go`) — `ShiftsRepo`.
- `internal/service/shift.go` (create; supersedes `note.go`) — `ShiftService` + suggestion/aggregate.
- `internal/service/invoice.go` (modify) — status cascade to shifts on create/status/delete.
- `internal/agent/tools_shifts.go` (create; supersedes `tools_notes.go`) — `list_participant_shifts` (+candidates), keep `search_catalogue`.
- `internal/agent/tools_invoice.go` (modify) — verify/bill keyed on shifts; `extract` not here.
- `internal/agent/extract.go` (create) — timesheet text → structured shifts (LLM call) + tool/endpoint.
- `internal/http/shifts.go` (create) — shift CRUD + filters + import + draft-invoice (move from agent.go or keep).
- `internal/http/server.go`, `main.go` (modify) — wire shift handler/tools; remove notes wiring.

**Frontend**
- `web/src/lib/api/shifts.ts`, `web/src/lib/stores/shifts.svelte.ts` (create; supersede notes).
- `web/src/routes/+page.svelte` (rewrite) — Shifts home (table + pipeline + to-record + suggestions).
- `web/src/routes/calendar/+page.svelte` (create) — month calendar.
- `web/src/routes/participants/[id]/+page.svelte` (create) — participant profile.
- `web/src/lib/components/ShiftTable.svelte`, `ShiftForm.svelte`, `Calendar.svelte`, `InvoiceSuggestions.svelte` (create).
- `web/src/routes/+layout.svelte` (modify) — nav (remove Assistant), remove ⌘K/agentChat.
- Remove: `web/src/lib/components/agent/*` chat UI, `routes` chat home, `NotesJournal.svelte`, notes api/store.

---

## Phase 1 — Backend: shifts schema + repo + service

### Task 1.1: shifts migration + sqlc
**Files:** Create `internal/db/migrations/00004_shifts.sql`, `internal/db/queries/shifts.sql`; regenerate `internal/db/gen`.

- [ ] **Step 1:** Write migration `00004_shifts.sql` (+goose Up/Down). Create `shifts` ALONGSIDE `notes` (keep notes so the build stays green; notes removed in Phase 5 cleanup once consumers migrate):
  `id, uuid, tenant_id→tenants, participant_id→participants, service_date TEXT, start_time TEXT, end_time TEXT, hours REAL, km REAL, measures TEXT DEFAULT '[]', note TEXT DEFAULT '', tags TEXT DEFAULT '[]', status TEXT NOT NULL DEFAULT 'recorded' CHECK(status IN ('scheduled','recorded','drafted','sent','paid')), invoice_id INTEGER REFERENCES invoices(id) ON DELETE SET NULL, author_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, created_at, updated_at`. Indexes on `(tenant_id,participant_id,service_date)`, `(tenant_id,status)`, `(invoice_id)`. Down: drop shifts.
- [ ] **Step 2:** Write `queries/shifts.sql`: `ListShifts`(tenant), `ListShiftsByParticipant`, `ListShiftsByParticipantRange`, `ListShiftsByStatus`, `ListScheduledShifts`(status='scheduled'), `GetShift`, `CreateShift`, `UpdateShift`, `UpdateShiftStatus`, `SetShiftInvoice`(invoice_id+status), `ClearShiftsForInvoice`, `DeleteShift`, `ParticipantUnbilledAgg` (group recorded unbilled by participant: count, min/max date).
- [ ] **Step 3:** Run `"$(go env GOPATH)/bin/sqlc" generate`; verify `internal/db/gen/shifts.sql.go` exists, `go build ./internal/db/...`.
- [ ] **Step 4:** Commit `feat(db): shifts table + queries (supersede notes)`.

### Task 1.2: ShiftsRepo (TDD)
**Files:** Create `internal/repository/shift.go`, `internal/repository/shift_test.go`. Model on committed `note.go`.

- [ ] **Step 1:** Write `shift_test.go`: Create round-trips all fields incl JSON `measures`/`tags`, status default `recorded`, ISO-date validation, tenant isolation; Get; ListByParticipant range; UpdateStatus; SetInvoice (status→drafted, invoice_id set); ClearForInvoice; negative hours/km rejected.
- [ ] **Step 2:** Run tests → fail.
- [ ] **Step 3:** Implement `ShiftsRepo` (audited via `audit.WithTx`; JSON-encode measures/tags; `validISODate`; helpers from note.go). Domain `Shift`/`ShiftInput` structs (camelCase json).
- [ ] **Step 4:** `go test ./internal/repository/ -run Shift -race` → pass; `go vet`.
- [ ] **Step 5:** Commit `feat(repo): ShiftsRepo with lifecycle + JSON measures/tags`.

### Task 1.3: ShiftService + suggestions (TDD)
**Files:** Create `internal/service/shift.go`, `internal/service/shift_test.go`. Add invoice-ownership check (as `NoteService.Bill` had).

- [ ] **Step 1:** Write `shift_test.go`: Create broadcasts SSE `shift`/`create`; ListParticipant range; UpdateStatus broadcasts; `Suggestions()` clusters recorded-unbilled by participant (count, from/to, ids); `ToRecord()` returns scheduled; `MarkDrafted(invoiceID, shiftIDs)` verifies invoice belongs to tenant then sets status+invoice.
- [ ] **Step 2:** Run → fail.
- [ ] **Step 3:** Implement `ShiftService` (reqctx tenant/user, hub broadcast entity `"shift"`, holds `*repository.InvoicesRepo` for ownership check).
- [ ] **Step 4:** `go test ./internal/service/ -run Shift -race` → pass; vet.
- [ ] **Step 5:** Commit `feat(service): ShiftService + invoice suggestions + lifecycle`.

### Task 1.4: invoice→shift status cascade
**Files:** Modify `internal/service/invoice.go` (+ test).

- [ ] **Step 1:** Test: on `UpdateStatus(sent|paid)` the invoice's shifts advance to sent/paid; on `Delete` shifts revert (FK clears invoice_id → recorded). Use a `ShiftsRepo` dependency or a thin hook.
- [ ] **Step 2-4:** Implement minimal cascade (InvoiceService gains `*repository.ShiftsRepo`; update shift statuses by invoice_id within the same flow), test, vet.
- [ ] **Step 5:** Commit `feat(service): cascade invoice status to its shifts`.

**Phase 1 gate:** `go test -race ./internal/repository/ ./internal/service/`, `go vet ./...`, `CGO_ENABLED=0 go build .` — all green.

---

## Phase 2 — Agent: shift-keyed tools + extraction + suggestions

### Task 2.1: shift tools (TDD)
**Files:** Create `internal/agent/tools_shifts.go` (+ test). Supersede `tools_notes.go`.

- [ ] `NewListParticipantShiftsTool(svc)` + `…WithCatalog` (candidates) — mirror committed notes tools but over shifts; body→note fenced via `wrapUntrusted`. Keep `search_catalogue`.
- [ ] Tests mirror `tools_notes_test.go`/`tools_notes_candidates_test.go`; commit.

### Task 2.2: create_invoice keyed on shifts
**Files:** Modify `internal/agent/tools_invoice.go` (+ tests).

- [ ] Repurpose `verifyNotesCovered`/`billCoveredNotes` → shifts: verify recorded shifts in range are covered as coded lines; after create, `MarkDrafted` covered shifts. `notesFrom/notesTo` → `from/to`. Update `tools_invoice_create_test.go`/`tools_invoice_verify_test.go`.
- [ ] Commit.

### Task 2.3: timesheet → shifts extraction (TDD)
**Files:** Create `internal/agent/extract.go` (+ test).

- [ ] `ExtractShifts(ctx, text) ([]ShiftDraft, error)` — one LLM call with a structured-output schema (participant name, [{date,start,end,hours,km,note}]); deterministic post-validate (ISO dates, non-negative). Unit test with `llm.Fake` scripting the structured response; gated live test parsing `message.txt` → 4 shifts.
- [ ] Commit.

**Phase 2 gate:** `go test -race ./internal/agent/` (skip live) green; vet; build.

---

## Phase 3 — HTTP: shift endpoints + import + draft-invoice + cascade

### Task 3.1: ShiftHandler (TDD)
**Files:** Create `internal/http/shifts.go` (+ test); wire `internal/http/server.go`, `main.go`.

- [ ] Routes: `GET /participants/{id}/shifts?from&to&status`, `POST /shifts`, `PUT /shifts/{id}`, `DELETE /shifts/{id}`, `POST /shifts/{id}/status`, `GET /shifts` (all, for the table), `GET /shifts/suggestions`.
- [ ] Tests mirror committed `notes_test.go`. Commit.

### Task 3.2: import + draft-invoice
**Files:** Modify `internal/http/shifts.go` / `agent.go`.

- [ ] `POST /shifts/import` `{text}` → `ExtractShifts` → create recorded shifts → return them. `POST /participants/{id}/draft-invoice` (already prototyped in `agent.go`) keyed on shifts (auto-approve, returns invoice). Tests (gated live for import). Commit.

**Phase 3 gate:** full offline `go test -race ./...` green; build.

---

## Phase 4 — Frontend: rebuild IA, remove chat

### Task 4.1: install table package + api/store
- [ ] `cd web && npm i @careswitch/svelte-data-table`. Create `api/shifts.ts`, `stores/shifts.svelte.ts` (mirror notes; add status/suggestions/import/draftInvoice calls). Commit.

### Task 4.2: remove chat UI
- [ ] Delete `components/agent/ChatPane|ConversationList|Composer` (+ chat-only), the chat home `+page.svelte`, `NotesJournal.svelte`, notes api/store. Strip Assistant nav + ⌘K + agentChat from `+layout.svelte`. Keep `api/agent.ts`. `npm run check` clean. Commit.

### Task 4.3: Shifts home
- [ ] `routes/+page.svelte` = pipeline strip + to-record prompts + quick-add (paste→import) + `InvoiceSuggestions` + `ShiftTable` (`@careswitch/svelte-data-table`, per-column sort/search/filter). `ShiftForm.svelte` modal (semi-structured: date, participant, start/end→hours, km, +measure, note; AI tags). Commit.

### Task 4.4: Calendar + Participant profile
- [ ] `routes/calendar/+page.svelte` (month, status colours, click-day add/record). `routes/participants/[id]/+page.svelte` (shifts/calendar/invoices + suggestion). Commit.

### Task 4.5: Invoice detail + status actions
- [ ] Invoice detail shows source shifts + Mark sent/paid (cascades). Draft-invoice button → lands on invoice. Commit.

**Phase 4 gate:** `npm run check` 0/0, `npm run build`, `npm run test` green.

---

## Phase 5 — Polish + end-to-end

- [ ] Per-column filter wiring, pipeline counts, nav to-record badge, accessibility (aria-live/labels), empty states.
- [ ] Full gate: `go test -race ./...`, `go vet`, `gofmt -l`, `CGO_ENABLED=0 go build .`, `cd web && npm run check && npm run build && npm run test`.
- [ ] Gated live: `RUN_LIVE_AGENT=1` import `message.txt` → 4 shifts → draft invoice → $1905.76.
- [ ] Commit; finishing-a-development-branch.

---

## Notes for the implementer
- Clean-break, but staged: add `shifts` alongside `notes` so every phase builds; migrate consumers (agent/http/main/frontend) phase by phase; **remove `notes` (table + Go + frontend) only in Phase 5 cleanup**. No prod data to preserve.
- Reuse committed harness exactly; this is a rename/evolution (notes→shifts) plus lifecycle + extraction, not a rewrite of pricing/verify.
- Every DB mutation audited + SSE broadcast after commit (entity `"shift"`); JSON camelCase; lists return `[]`.
- Each task TDD: failing test → minimal impl → green → commit. Ignore stale LSP `gen.*` errors; trust `go build`/`go test`.
