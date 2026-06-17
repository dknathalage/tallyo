# Shifts Lifecycle + AI Invoicing — Product Redesign

**Date:** 2026-06-17
**Status:** Approved (design); planning → implementation
**Prototype:** `.superpowers/brainstorm/51943-1781699403/prototype-v6.html`

## Vision

Tallyo's primary job: a support **worker = owner** (single user for now) records the
support **shifts** they delivered, and the app — with AI — turns recorded shifts
into NDIS invoices. The real-world input is a free-text timesheet message (see
`message.txt`); the app makes capturing and billing it effortless. The AI chat is
no longer the centre and is **removed from the UI** (the agent backend/harness
stays, repurposed behind buttons).

## The core entity: Shift

Evolve the existing `notes` table into **shifts** (a note becomes one *field* of a
shift). A shift is semi-structured:

- `participant_id`, `service_date`
- `start_time`, `end_time` (optional; derive hours), `hours` (REAL), `km` (REAL)
- `measures` — optional extra quantities beyond hours/km (JSON list of
  `{label, value, unit, code?}`) for "other measures"
- `note` — free text (one field; was `body`)
- `tags` — catalog codes (NDIS support item codes) the AI auto-assigns; user-editable
- `status` — lifecycle: `scheduled → recorded → drafted → sent → paid`
- `invoice_id` — set when drafted (was `billed_invoice_id`)

### Status lifecycle
- **scheduled** — planned (today/upcoming/overdue); no actuals yet. App prompts to record.
- **recorded** — actuals entered (hours/km/measures/note); billable.
- **drafted** — placed on a draft invoice.
- **sent** — its invoice was sent.
- **paid** — its invoice was paid.

Invoice status changes cascade to its shifts (draft→drafted, sent→sent, paid→paid;
delete/revert → back to recorded, FK `ON DELETE SET NULL` clears `invoice_id`).

## AI capabilities (agent backend reused, no chat UI)

1. **Timesheet → shifts (NEW):** parse a pasted free-text message into multiple
   recorded shifts (participant match, per-day date/time/hours/km/note), auto-tagged.
2. **Auto-tag catalog codes (NEW):** assign NDIS codes to a shift's measures
   (hours→self-care class, km→transport, etc.) — deterministic where possible,
   AI for ambiguous; user can override.
3. **Invoice suggestions (NEW):** cluster `recorded` unbilled shifts by participant
   (and timeframe) and surface "draft invoice" suggestions.
4. **Draft invoice (reuse harness):** from a suggestion (or chosen shifts), the AI
   selects shifts, maps codes, applies catalogue-authoritative prices, runs the
   completeness verify, and creates a **draft** invoice — auto-approved (no chat,
   no human gate), landing the user on the invoice. Pillars 1/4 carry over;
   shifts↔invoice replaces notes↔invoice.

## Frontend IA (chat removed)

Nav: **Shifts · Calendar · Participants · Invoices · Catalog · Settings.**

- **Shifts** (home): pipeline strip (counts per stage); "shifts to record" prompt
  (scheduled today/overdue/upcoming); quick-add (paste timesheet → AI extract);
  "+ Ad-hoc shift"; AI invoice suggestions; a flat **sortable / searchable /
  per-column-filterable table** (Date, Participant, Time, Hrs, Km, Note, Tags,
  Status). Table package: **`@careswitch/svelte-data-table`** (Svelte 5, runes).
- **Calendar** (month): shifts colour-coded by status; click a day → add/record a
  shift; click a shift → record/edit.
- **Participants → profile** (`/participants/{id}`): the participant's shifts,
  calendar, invoices; ad-hoc add; per-participant invoice suggestion.
- **Invoices**: list → invoice detail (line items, **source shifts**, status
  actions Mark sent / Mark paid).
- **Recording form** (modal): semi-structured — date, participant, start/end
  (auto hours), km, + add measure, note; AI auto-tags on save.

### Removed
The assistant chat: `/` chat page, `components/agent/*` chat UI, the Assistant nav
entry, ⌘K conversation shortcut, `agentChat` layout wiring. Keep `api/agent.ts`
plumbing and the agent backend.

## Backend changes

- **Migration** `00004`: evolve `notes` → `shifts` (add start/end, status, tags,
  measures, rename body→note, billed_invoice_id→invoice_id). Clean-break allowed
  (no production data) — may recreate the table.
- **Repository/service**: `ShiftsRepo`/`ShiftService` (evolve notes repo/service):
  CRUD, list/filter, status transitions, `MarkBilled`/cascade, dashboard/
  suggestion aggregates, scheduled-to-record queries.
- **Agent tools**: `list_participant_shifts` (+ candidates), `create_invoice`
  (already catalogue-authoritative + verify) keyed on shifts; new
  `extract_shifts_from_text` capability; `search_catalogue` reused.
- **HTTP**: shift CRUD + range/status filters; `POST /participants/{id}/draft-invoice`
  (auto-approve, returns invoice) — already prototyped; `POST /shifts/import`
  (paste text → shifts); invoice status transitions cascade to shifts.

## Testing

- Repo/service: shift CRUD, status transitions + invoice cascade, ISO-date
  validation, tenant isolation, suggestion/aggregate queries, "to-record" query.
- Agent: timesheet→shifts extraction (deterministic chain + gated live), auto-tag,
  draft-invoice auto-approve over shifts, completeness verify.
- HTTP: shift endpoints, draft-invoice, status cascade, authz/tenant.
- Frontend: shifts table (sort/filter), recording form (time→hours), calendar
  create, draft→invoice landing, status pipeline; remove chat tests.

## Phasing (the plan will detail)

1. Backend: notes→shifts schema + repo/service + status lifecycle + tests.
2. Agent: shift-keyed tools + timesheet→shifts extraction + auto-tag + suggestions.
3. HTTP: shift endpoints + draft-invoice + import + status cascade.
4. Frontend: remove chat; Shifts table; recording form; calendar; participant
   profiles; invoices detail + status actions; wire AI buttons.
5. Polish: pipeline strip, to-record prompts, per-column filters, package
   integration, accessibility, end-to-end verify.

## Out of scope (for now)

Multi-user worker/owner separation + approval queues; rostering/import of
scheduled shifts from external systems (scheduled shifts entered manually for
now); SMS/email inbound ingestion.
