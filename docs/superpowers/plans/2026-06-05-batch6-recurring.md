# Domain Port — Batch 6: recurring_templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port recurring invoice templates — CRUD plus an **idempotent scheduled sweep** that generates invoices from due templates (run-on-launch + in-session ticker), advancing `next_due` atomically with each generation.

**Architecture:** Standard per-domain pattern + a generator. A template stores `client_id`, `frequency` (weekly/monthly/quarterly), `next_due`, `line_items` (JSON array), `tax_rate` (percent), `notes`, `is_active`. The sweep selects active templates with `next_due <= today` and, for EACH, in ONE `numbering.WithRetry` transaction: generate an invoice (number via `numbering.Invoice`, default snapshots from business_profile/client, totals tax-as-percent — reusing invoice.go helpers), insert invoice + line items, advance `next_due` by the frequency, and audit — all atomic so a crash never double-generates and re-running is safe (idempotent: `next_due` advances past today, and the select only takes `next_due <= today`).

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md` (Scheduling section)

**Reference templates:** `internal/repository/invoice.go` (reuse its private helpers: `round2`, snapshot builders, `computeTotals`, gen.CreateInvoice/CreateLineItem — same package), `internal/numbering`, Batch-1/5 repo/service/handlers, `cmd/tallyo/main.go` (the existing overdue ticker — copy that pattern for the recurring sweep).

**Schema (verbatim, clean-break):**
```sql
recurring_templates: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE,
  client_id INTEGER REFERENCES clients(id) ON DELETE SET NULL, name TEXT NOT NULL,
  frequency TEXT NOT NULL, next_due TEXT NOT NULL, line_items TEXT NOT NULL DEFAULT '[]',
  tax_rate REAL NOT NULL DEFAULT 0, notes TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
  -- indexes: idx_recurring_client(client_id), idx_recurring_next_due(next_due)
```

**Frequency advance (port of advanceNextDue, date "YYYY-MM-DD"):** weekly → +7 days; monthly → +1 month; quarterly → +3 months. (Only these three. Use Go `time.Parse("2006-01-02", d)` then `AddDate`.)

---

## Task 1: Migration 00008 + sqlc queries

**Files:** `internal/db/migrations/00008_recurring.sql`; `internal/db/queries/recurring_templates.sql`; regen; migration test.

- [ ] **Step 1: Migration** — recurring_templates + the 2 indexes. Down drops the table.
- [ ] **Step 2: sqlc** (lists LEFT JOIN clients for client_name like invoices):
  - `ListRecurringTemplates` (LEFT JOIN clients, ORDER BY next_due), `ListActiveRecurringTemplates` (WHERE is_active=1), `GetRecurringTemplate` (join, by id), `ListDueTemplates` (`WHERE is_active=1 AND next_due <= ? ORDER BY next_due` — pass today), `CreateRecurringTemplate` (RETURNING *), `UpdateRecurringTemplate` (RETURNING *), `SetRecurringNextDue` (`UPDATE recurring_templates SET next_due=?, updated_at=? WHERE id=?`), `DeleteRecurringTemplate`.
- [ ] **Step 3: Generate + build.** REPORT gen `RecurringTemplate` struct (is_active → int64; tax_rate → float64; client_id → sql.NullInt64; line_items/name/frequency/next_due/notes → string or NullString) + join row (+ client_name) + Create/Update Params + ListDueTemplates param (the today string).
- [ ] **Step 4: Migration test** — `TestMigrateCreatesRecurringTable`.
- [ ] **Step 5: Run** db tests, vet, gofmt → version 8. **Commit** `feat(db): recurring_templates migration and sqlc queries`.

---

## Task 2: Recurring repository (CRUD + idempotent generator)

**Files:** Create `internal/repository/recurring.go` (+ `_test.go`).

- [ ] **Step 1: Failing test** (seed business_profile + client). Cover:
  - CRUD: Create(ctx, RecurringInput) → *RecurringTemplate (audited "recurring_template"/"create"); validate name non-empty, client present, frequency in {weekly,monthly,quarterly}; List/ListActive; Get(nil-missing); Update; Delete; toggle is_active via Update.
  - `AdvanceDate(date, freq)` helper: weekly/monthly/quarterly produce the right dates.
  - `GenerateOne(ctx, templateID) (*Invoice, error)`: creates a draft invoice from the template's line_items + tax_rate (number INV-xxxx, default snapshots, totals tax%) AND advances the template's next_due — atomically. Returns the new invoice.
  - `GenerateDue(ctx) ([]GeneratedInvoice, error)`: seed a template with `next_due` = yesterday (active) and one with `next_due` = tomorrow → only the due one generates; returns its {templateId, invoiceId, invoiceNumber}; the template's next_due is advanced past today; **idempotency:** calling GenerateDue AGAIN immediately returns empty (the template is no longer due). Also assert a generated invoice exists with the template's line items + computed total.
  - (Optional) a concurrency note: not strictly required, but the per-template generate uses numbering.WithRetry so concurrent sweeps are safe.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/recurring.go`:
  - Domain `RecurringTemplate{ID int64; UUID string; ClientID *int64; ClientName string; Name, Frequency, NextDue string; LineItems []*RecurringLine; TaxRate float64; Notes string; IsActive bool; CreatedAt, UpdatedAt string}` (json camelCase: clientId, clientName, nextDue, lineItems, taxRate, isActive). `RecurringLine{Description string; Quantity, Rate float64; Notes string; SortOrder int64}` (the JSON shape stored in line_items). `RecurringInput{ClientID *int64; Name, Frequency, NextDue string; LineItems []RecurringLine; TaxRate float64; Notes string; IsActive bool}`.
  - `RecurringRepo{db}`, `NewRecurring(db)` panic-if-nil. Hold helpers access to invoice.go (same package).
  - List/ListActive/Get/Create/Update/Delete — standard audited CRUD. line_items stored as a JSON string (`json.Marshal` the []RecurringLine on write; unmarshal on read). Validate name + frequency ∈ {weekly,monthly,quarterly} + clientId present on create.
  - `AdvanceDate(date, freq string) (string, error)`: parse + AddDate per frequency; error on unknown frequency.
  - `GenerateOne(ctx, templateID) (*Invoice, error)`: load template (nil-missing → (nil,nil)); parse line items → []LineItemInput; build default snapshots (business_profile + client; payer "{}"); compute totals (tax% — reuse computeTotals/round2); in `numbering.WithRetry(ctx, 10, func(){ tx:=BeginTx; num:=numbering.Next(ctx,tx,numbering.Invoice); insert invoice (client_id, date=today, due_date=today, totals, tax_rate, notes, status draft, snapshots) via gen.CreateInvoice; insert line items; newNextDue,_ := AdvanceDate(template.NextDue, template.Frequency); SetRecurringNextDue(templateID, newNextDue, now); audit.Log(tx, "invoice"/"create"/invID, context "from recurring template <name>"); commit; capture invID })`; return the new invoice via NewInvoices(db).Get OR build it. (The next_due advance is in the SAME tx as the invoice insert — critical for idempotency.)
  - `GenerateDue(ctx) ([]GeneratedInvoice, error)`: today := time.Now().UTC().Format("2006-01-02"); due := ListDueTemplates(today); for each, call GenerateOne(template.ID) (each its own tx); collect {TemplateID, InvoiceID, InvoiceNumber}. Return them. (Empty when none due.)
  - `GeneratedInvoice{TemplateID, InvoiceID int64; InvoiceNumber string}`.
  - Keep funcs <60 lines — extract `generateInvoiceTx`, `parseLines`.

- [ ] **Step 4: Run** `go test ./internal/repository/ -race` (incl idempotency: GenerateDue twice → second is empty), vet, gofmt, build. **Commit** `feat(repository): recurring templates with idempotent invoice generation`.

---

## Task 3: Recurring service + handlers + sweep wiring

**Files:** Create `internal/service/recurring.go`, `internal/http/recurring.go` (+ tests). Modify `server.go`, `cmd/tallyo/main.go`.

- [ ] **Step 1: Service** — `RecurringService{repo, hub}`. CRUD methods (broadcast `recurring_template`/<id>/<action>). `GenerateOne(ctx, id)` → broadcast `{recurring_template, id, "generate"}` + `{invoice, invID, "create"}`. `GenerateDue(ctx) ([]GeneratedInvoice, error)` → if any, broadcast `{invoice, 0, "recurring_sweep"}`; return them. Tests: create broadcasts; GenerateOne broadcasts recurring+invoice.

- [ ] **Step 2: Handlers** (behind RequireAuth): `GET /api/recurring` (?active=), `POST /api/recurring`, `GET/PUT/DELETE /api/recurring/{id}`, `POST /api/recurring/{id}/generate` (200 + the generated invoice). Validation (name/frequency/client → 400). Tests (seed client): CRUD + generate (creates an invoice, advances next_due).

- [ ] **Step 3: Wire** server.go (Deps.Recurring + guard + routes) + cmd/tallyo:
  - Construct recurringSvc + handler into Deps.
  - **Launch sweep + ticker:** run `recurringSvc.GenerateDue(ctx)` once at startup (log count). Add a second bounded `time.Ticker` (or reuse a shared sweeper goroutine) that calls GenerateDue periodically (e.g. hourly — can share the overdue ticker's interval/goroutine: have ONE sweeper goroutine that calls both MarkOverdue and GenerateDue each tick). Stop cleanly on shutdown (reuse the existing `overdueDone`/ticker shutdown pattern — extend it to also run the recurring sweep, OR add a parallel one; keep it leak-free).

- [ ] **Step 4: Run** `go test ./... -race`, vet, gofmt; boot smoke (create client+template with next_due=yesterday → at startup OR via POST generate, an invoice is generated; GET /api/invoices shows it; GET /api/recurring shows next_due advanced; POST generate again advances again). **Commit** `feat(recurring): service, REST endpoints, scheduled generation`.

---

## Task 4: Frontend — recurring templates UI

**Files:** store `web/src/lib/stores/recurring.svelte.ts`; route `web/src/routes/recurring/+page.svelte`; types; nav.

- [ ] **Step 1: Types + store** — RecurringTemplate, RecurringLine, RecurringInput (camelCase). `createCollectionStore('recurring','recurring_template')`.
- [ ] **Step 2: Route** — list (name, client, frequency, nextDue, active badge); create/edit form: client select, name, frequency select (weekly/monthly/quarterly), nextDue date, tax-rate select, notes, a line-items editor (description/quantity/rate like invoices), is_active toggle. A "Generate now" button per template → `apiPost('/api/recurring/'+id+'/generate',{})` then reload (show the new invoice number). Delete. Client-side search.
- [ ] **Step 3: Nav** — add "Recurring" link.
- [ ] **Step 4: Verify** `npm run check` (0/0), build (200.html), `touch build/.gitkeep`. **Commit** `feat(web): recurring templates UI with generate-now`.

---

## Task 5: Batch 6 acceptance

- [ ] **Step 1: Gates** — `go test ./... -race`, vet, gofmt, `npm run check` + build.
- [ ] **Step 2: Live smoke** — boot (wait ready); setup+login; seed client+tax; create a recurring template with next_due=yesterday; POST /api/recurring/{id}/generate → an invoice appears (INV-xxxx, totals from the template's line items), template next_due advanced; generate again → another invoice + next_due advances again; toggle is_active off → due-but-inactive not swept; SSE `recurring_template`/create + `invoice`/create events. Capture output.
- [ ] **Step 3: Commit** `chore: batch 6 acceptance — recurring templates full-stack`.

---

## Done When

- recurring_templates migrated; CRUD; the sweep generates invoices from due active templates and advances next_due ATOMICALLY (idempotent — re-running doesn't double-generate); launch + ticker scheduling, leak-free shutdown; manual generate endpoint; mutations audited + broadcast.
- Frontend templates UI with frequency/line-items/active + generate-now.
- All gates green; live smoke confirms generation, next_due advance, idempotency, SSE.

Batch 7 (PDF) renders invoices/estimates to PDF via maroto. Batch 8 (import/export) is the final domain.
