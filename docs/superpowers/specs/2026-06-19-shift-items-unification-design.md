# Shift items = invoice line items — design

**Date:** 2026-06-19
**Status:** approved, pending implementation plan

## Problem

A shift records a billable quantity **three** ways: `hours` (REAL), `km` (REAL),
and `measures` (JSON `{label,value,unit,code}`). The agent special-cases hours and
km with **hardcoded** catalogue codes (`hours → 01_011_0107_1_1`,
`km → 04_590_0125_6_1`) when drafting an invoice, and `verifyShiftsCovered`
reconciles shift quantities against the lines it produced. Three representations +
hardcoded mapping + a reconciliation step = the complexity to remove.

## Goal

One uniform concept: a shift carries **items**, and **a shift item is an invoice
line item — the same row**. Users add only the items that apply (empty shift =
note only). The AI's job shrinks to *dividing a shift's note/inputs into items*;
users can divide manually too. Drafting an invoice stops transforming anything —
it links existing items to an invoice.

## Design

### 1. One table (`line_items`)

`line_items` becomes the single home for shift items and invoice lines.

- **add** `shift_id INTEGER NULL REFERENCES shifts(id) ON DELETE CASCADE` (G2):
  the shift service **blocks deleting a shift whose status is past `recorded`**
  (drafted/sent/paid = billed), so a billed shift's items can never be deleted;
  an unbilled shift's items cascade away cleanly. Simpler than SET-NULL +
  delete-unbilled, and it can't violate the G1 CHECK via a participant cascade
  (participant delete is already RESTRICTed by `invoices.participant_id`).
- **change** `invoice_id` from `NOT NULL` → **NULL** (NULL = unbilled shift item)
- **add** `start_time TEXT NULL`, `end_time TEXT NULL` (time-class units only)
- **add** `CHECK (shift_id IS NOT NULL OR invoice_id IS NOT NULL)` (G1) — no
  orphan row; every line belongs to a shift, an invoice, or both.

Lifecycle of one row: created on a shift (`shift_id` set, `invoice_id` NULL) →
draft sets `invoice_id` → row never copied. Recurring/manual invoice lines are
born with `shift_id` NULL, `invoice_id` set — the CHECK permits both shapes.

`shifts`: **drop `hours`, `km`, `measures`** (and the now-unused `start_time`/
`end_time` shift columns — time lives on the item). Keep `service_date`,
`participant_id`, `note`, `tags`, `status`, `invoice_id`, `author_user_id`.
`shifts.status` + `shifts.invoice_id` stay for the lifecycle cascade; items carry
their own `invoice_id` so invoice reads find them.

### 2. Unit class drives input

Unit comes from the catalogue `unit_of_measure` (`H`, `KM`, `EA`, `D`, `WK`, …).
A single `classify(unit)` function maps a unit string to an input class:

| class | units | quantity from |
|---|---|---|
| time | `H` / hour | `start_time` + `end_time` → duration |
| distance | `KM` | typed distance |
| count | everything else | typed number |

No user-defined units. The form renders the quantity input from the class.
`ponytail:` classify is a small switch — extend when a new unit class appears.

### 3. AI role shrinks

The draft-invoice Smart is repurposed: instead of inventing invoice lines, it
**divides a shift** (note + any existing items) into priced, coded `line_items`
rows with `shift_id` set and `invoice_id` NULL — resolving catalogue code + price
**once**, here. `verifyShiftsCovered` is **deleted**; "coverage" is now just
whether the shift has items, directly visible. Users can add/edit/delete items
manually via the form (a catalogue picker) without the AI.

### 4. Draft = pure linking

Draft from N shifts → set `invoice_id` on those shifts' line items, set shift
`status = 'drafted'`, compute invoice totals from the now-linked lines. No
transformation, no copy. `ShiftLinker` / `MarkDrafted` simplify accordingly.

### 5. Invoice read guard

`invoice_id` is now nullable, so invoice reads must never see unbilled items.
Invoice line reads filter `invoice_id = ?` (already true). Shift-only reads filter
`shift_id = ? AND invoice_id IS NULL`. Audit every `line_items` query in
`internal/db/queries/*.sql` for the nullable change; totals/render only see rows
with `invoice_id` set.

### 6. Frontend

`web/src/lib/components/ShiftForm.svelte`: replace the `hours`/`km`/`measures`
inputs with an **items editor**:
- "Add item" → catalogue picker (sets label / unit / code) or a custom item.
- The item's unit class renders its quantity input (time → start/end pickers;
  distance → km field; count → number).
- A "Divide with AI" action populates items from the note.
- Empty shift = note only; add only items that apply.

### 7. Migration / existing data

Drop `hours` / `km` / `measures` columns with **no backfill** — early stage; notes
survive, so the AI (or user) can re-divide old shifts. Backfilling would
re-introduce the hardcoded hours→code / km→code mapping being deleted.

### 8. Integrity & edge cases (resolved gaps)

- **G1 — orphan rows.** `CHECK (shift_id IS NOT NULL OR invoice_id IS NOT NULL)`
  on `line_items` (see §1).
- **G2 — shift/invoice delete vs invoiced lines.** `shift_id ON DELETE CASCADE`;
  the shift service refuses to delete a shift past `recorded` status (its items are
  on an invoice). Invoice delete (`invoice.Service.Delete`) first runs
  `UnlinkShiftItemsFromInvoice` (set `invoice_id` NULL where `shift_id` is set), so
  shift items return to their shift; the FK cascade then removes only the manual
  (shift-less) lines. `invoice_id` keeps `ON DELETE CASCADE`.
- **G3 — price pin timing.** `unit_price` + `catalog_version_id` resolve at
  **item-creation** keyed off the item's `service_date` (the effective catalogue
  for that date), **not** the wall-clock creation time and **not** re-resolved at
  draft. An item priced on a shift keeps that price through drafting. `ponytail:`
  pinned-at-creation; re-price only if a user edits the item's date/quantity.
- **G4 — `service_date` duplicated** on shift + line. The line snapshots it. When
  a shift's `service_date` is edited, the shift service updates `service_date`
  (and re-pins price per G3) on that shift's **unbilled** items in the same tx.
  Billed items are frozen.
- **G5 — empty-shift draft.** Drafting requires ≥1 item; a note-only shift cannot
  be drafted (handler returns a validation error). Note-only shifts are valid to
  record, just not billable.
- **G6 — `sort_order` is invoice-only.** Set at draft to order lines within the
  invoice. The shift items editor orders by `id` (creation order); it does not use
  `sort_order`. `ponytail:` one column, one owner — revisit only if shifts need
  manual item reordering.

## Out of scope

- `estimate_line_items` stays a separate table. Estimates are unaffected in
  behaviour, but the estimate→invoice conversion mapper shares the generated
  `CreateLineItem` params, so it gets a mechanical update for the new columns.
- No change to catalogue versioning / price pinning.
- No change to invoice immutability once sent.

## Affected code (orientation, not exhaustive)

- `internal/db/migrations/00008_*.sql` (new), `internal/db/queries/{line_items,shifts}.sql`, `sqlc generate`
- `internal/shift/{repository,service,handler}.go` — drop hours/km/measures; item CRUD on shift
- `internal/billing/lineitem.go` — `ShiftID *int64`, nullable `InvoiceID`, `StartTime`/`EndTime`
- `internal/agent/smart_draft_invoice.go` — repurpose to "divide shift"; delete `verifyShiftsCovered`
- `internal/invoice/service.go` — draft = link; totals from linked lines
- `web/src/lib/components/ShiftForm.svelte`, `web/src/lib/api/{types,shifts}.ts`
