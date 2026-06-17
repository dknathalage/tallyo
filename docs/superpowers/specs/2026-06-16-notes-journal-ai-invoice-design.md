# Notes Journal + AI Invoice-from-Notes — Design

**Date:** 2026-06-16
**Status:** Approved (design); implementing

## Problem

Support providers keep a daily journal of what they did with a participant
(receiver) — e.g. the reference nursing note for participant *Tania Hangevelled*,
week ended 14/6/2026. At the end of an arbitrary period they want an NDIS
invoice generated for that participant from those notes. The AI reads the notes,
maps each activity to an NDIS support item (code + rate) and drafts an invoice
for approval.

The reference fixture (`Invoice-2638907B.pdf`) is the acceptance target:
4 service days, two support items per day —

```
04_590_0125_6_1  Activity Based Transport          $1.00 / km    (GST-free)
01_011_0107_1_1  Assistance With Self-Care (wd)     $70.23 / hour (GST-free)

Tue 9/6   36 km   7.0 h
Wed 10/6  12 km   5.5 h
Thu 11/6  64 km   7.0 h
Fri 12/6  38 km   5.5 h
```

→ a GST-free invoice, subtotal/total **$1905.76**.

## Scope

Three layers; only the third is novel.

- **Prerequisite (out of scope):** the catalogue (`support_items` +
  `support_item_prices`) is populated via the existing XLSX importer. The
  official NDIS XLSX (`NDIS Support Catalogue 2025-26.xlsx`) uses per-state
  price columns + `Remote`/`Very Remote` and has no `National` column, while
  the importer expects `national`; a header/zone-mapping fix to the importer is
  a **separate** task. Tests seed the two codes directly.
- **Required dependency:** the agent must be able to look up codes/rates → new
  `search_catalogue` read tool.
- **The feature:** notes journal CRUD + AI invoice-from-notes orchestration.

## Data model — `notes` (migration `00003`)

```
notes
  id                INTEGER PK
  uuid              TEXT NOT NULL UNIQUE
  tenant_id         INTEGER NOT NULL → tenants(id)
  participant_id    INTEGER NOT NULL → participants(id)
  service_date      TEXT NOT NULL         -- YYYY-MM-DD, day support happened
  body              TEXT NOT NULL         -- free-text journal entry (UNTRUSTED)
  transport_km      REAL NULL             -- optional structured tag
  support_hours     REAL NULL             -- optional structured tag
  author_user_id    INTEGER NULL → users(id)
  billed_invoice_id INTEGER NULL → invoices(id) ON DELETE SET NULL  -- soft flag
  created_at        TEXT NOT NULL
  updated_at        TEXT NOT NULL
index notes_participant_date (tenant_id, participant_id, service_date)
```

- Tenant-scoped like every other entity; FK to `tenants`.
- Every mutation audited via `audit.WithTx`; service broadcasts SSE entity
  `"note"` after commit (commit-then-publish).
- `billed_invoice_id` is the **soft** billing flag: nullable, set when notes are
  billed, cleared (`ON DELETE SET NULL`) if the invoice is deleted. Never blocks.

## Layers (mirrors existing domains)

`internal/db/queries/notes.sql` (sqlc source) → `sqlc generate` →
`repository.NotesRepo` → `service.NoteService` (+SSE) →
`httpapi.NoteHandler` → wired into `main.go` `Deps` + router.

REST (camelCase JSON; list endpoints return `[]` non-nil when empty):

- `GET    /api/participants/:id/notes?from=&to=` — list (range optional)
- `POST   /api/notes`            — create `{participantId, serviceDate, body, transportKm?, supportHours?}`
- `PUT    /api/notes/:id`        — update
- `DELETE /api/notes/:id`        — delete
- `POST   /api/notes/bill`       — `{invoiceId, noteIds}` set `billed_invoice_id`

Queries: `CreateNote`, `GetNote`, `UpdateNote`, `DeleteNote`,
`ListParticipantNotes` (by participant), `ListParticipantNotesRange`
(participant + `[from,to]`), `MarkNotesBilled` (set invoice id for ids),
`ClearNotesBilledForInvoice` (unlink on invoice delete — or rely on FK SET NULL).

## AI flow — two new READ tools, reuse `create_invoice`

`internal/agent/tools_notes.go`:

- `NewListParticipantNotesTool(noteSvc)` — RiskRead, render `table`.
  Input `{participantId, from, to}` (dates `YYYY-MM-DD`). Returns note rows as
  structured JSON. The free-text `body` is the **untrusted** field and is passed
  through `wrapUntrusted("note-body", …)` when surfaced as standalone text, per
  the existing untrusted-content seam (`tools_invoice.go` header note).
- `NewSearchCatalogueTool(catalogSvc)` — RiskRead, render `table`.
  Input `{query, serviceDate}`. Resolves the catalogue version effective on
  `serviceDate`, returns matching support items (`code, name, unit, national
  price, gstFree`). This is what lets the AI map an activity description to a
  code + rate. (Add a `Search` method to `SupportCatalogService` if absent.)

Both registered in `main.go` alongside `list_invoices` / `create_invoice`.
`create_invoice` is unchanged (approval-gated, checkpointed, NDIS-validated).

**Orchestration.** The notes-page "Create invoice from notes" button seeds an
agent run with a templated prompt carrying the participant and the resolved
absolute `from`/`to` (the UI converts relative presets — this week / last week /
last N days / this month / custom — to dates). The agent:

1. `list_participant_notes(participantId, from, to)`
2. `search_catalogue(query, serviceDate)` per distinct activity
3. `create_invoice(...)` — user approves → draft invoice card

After the invoice exists, the **frontend** calls `POST /api/notes/bill` with the
note IDs it generated from. Billing-link is deterministic and frontend-driven;
the AI never owns billing state. Trade-off: if the AI bills only part of the
range, the link is approximate — acceptable under the soft flag.

## Frontend

- Participant notes view: journal list grouped by `service_date`; add/edit entry
  form (body + optional km/hours); `billed on INV-xxx` badge when billed.
- "Create invoice from notes" action with a date-range picker (relative presets
  + custom), which seeds the agent run and surfaces it in the existing chat pane.
- Built on existing SvelteKit SPA patterns (runes, SSE refetch, `api` client).

## Error handling

- Validation at boundaries (NASA rule 5): handler rejects missing
  `participantId`/`serviceDate`/`body`; non-negative `transportKm`/`supportHours`.
- Agent tools return structured tool errors (not panics) on bad input, mirroring
  `create_invoice`'s `is_error` contract.
- NDIS validation stays in `create_invoice`; a wrong code/over-cap price surfaces
  as the existing flattened validation message the agent can react to.

## Testing

- **Repository/service/handler** unit tests per existing style (temp migrated
  SQLite, seeded tenant + participant): CRUD, range filter, tenant isolation,
  `MarkNotesBilled`, SSE broadcast on mutation, `[]`-not-nil on empty list.
- **Agent tools**: `list_participant_notes` range filtering + untrusted-body
  wrapping; `search_catalogue` returns the correct code/national price for a
  service date and version.
- **Nursing-note fixture (acceptance):** seed the 4 days (Tania, 9–12 Jun) as
  `notes` rows + seed the two catalogue codes; assert the tools surface exactly
  the data `create_invoice` needs, and reuse the existing
  `TestCreateInvoiceFromNursingNote` proving the `$1905.76` invoice.
- **LLM extraction (honest limit):** deterministic tests cannot exercise the
  model's extraction/mapping. Add an optional, `ANTHROPIC_API_KEY`-gated
  end-to-end test (skipped in CI) that runs the real agent over the nursing-note
  fixture and asserts the resulting invoice totals.

## Out of scope / follow-ups

- NDIS XLSX importer header/zone mapping (state → national) — separate task.
- Hard billing locks, un-bill flows, multi-participant batch invoicing.
