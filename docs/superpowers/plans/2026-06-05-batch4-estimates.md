# Domain Port — Batch 4: estimates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port estimates + estimate_line_items full-stack — a near-clone of invoices (Batch 3) — plus the **convert-to-invoice** operation that writes into the invoices table.

**Architecture:** Mirror the invoice repo/service/handlers/UI exactly. Deltas vs invoices: number prefix `EST-` (`numbering.Estimate` config already exists), `valid_until` instead of `due_date`, **no** `payment_terms`, nullable `client_id`, extra `converted_invoice_id`, statuses `draft`/`accepted`/`declined`/`converted`, and a `Convert(estimateId)` method (accepted + not-yet-converted → generate an INVOICE via `numbering.Invoice`, copy snapshots/totals/line items, set `estimates.converted_invoice_id`). Same server-computed totals (tax as PERCENT, `tax_amount = round2(subtotal*rate/100)`), same editable-or-default snapshots, same numbering+WithRetry atomicity contract.

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md`

**Reference (THE template — copy and adapt):** `internal/repository/invoice.go` (+ test), `internal/service/invoice.go`, `internal/http/invoices.go`, `internal/db/migrations/00005_invoices.sql`, `internal/db/queries/{invoices,line_items}.sql`, `web/src/routes/invoices/+page.svelte`, `web/src/lib/stores/invoices.svelte.ts`. The invoice repo already proves numbering+snapshots+totals; estimates reuse all of it.

**Schema (verbatim, clean-break):**
```sql
estimates: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, estimate_number TEXT NOT NULL UNIQUE,
  client_id INTEGER REFERENCES clients(id), date TEXT NOT NULL, valid_until TEXT NOT NULL,
  subtotal REAL DEFAULT 0, tax_rate REAL DEFAULT 0, tax_rate_id INTEGER REFERENCES tax_rates(id) ON DELETE SET NULL,
  tax_amount REAL DEFAULT 0, total REAL DEFAULT 0, notes TEXT DEFAULT '', status TEXT DEFAULT 'draft',
  currency_code TEXT DEFAULT 'USD', converted_invoice_id INTEGER,
  business_snapshot TEXT DEFAULT '{}', client_snapshot TEXT DEFAULT '{}', payer_snapshot TEXT DEFAULT '{}',
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
  -- indexes: idx_estimates_status(status), idx_estimates_client_id(client_id)
estimate_line_items: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE,
  estimate_id INTEGER NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
  description TEXT NOT NULL, quantity REAL NOT NULL DEFAULT 1, rate REAL NOT NULL DEFAULT 0,
  amount REAL NOT NULL DEFAULT 0, notes TEXT DEFAULT '', sort_order INTEGER DEFAULT 0,
  catalog_item_id INTEGER, rate_tier_id INTEGER
  -- index: idx_estimate_line_items_estimate_id(estimate_id)
```
(NOTE: drizzle marks `quantity/rate/amount` with `.default()` only, not `.notNull()`, but the old invoice line_items used NOT NULL; use NOT NULL DEFAULT for consistency with the line_items table — they round-trip the same. `client_id` is nullable per drizzle; the repo validates it's present on Create.)

---

## Task 1: Migration 00006 + sqlc queries

**Files:** `internal/db/migrations/00006_estimates.sql`; `internal/db/queries/{estimates,estimate_line_items}.sql`; regen; migration test.

- [ ] **Step 1: Migration** — both tables (FKs: client_id→clients SET NULL or no-action? drizzle has no onDelete → default RESTRICT/no-action; use plain `REFERENCES clients(id)`; tax_rate_id SET NULL; estimate_id CASCADE; converted_invoice_id is a plain INTEGER, NO FK in drizzle — keep it a plain column). Indexes per schema. Down drops estimate_line_items then estimates.

- [ ] **Step 2: sqlc queries** (mirror invoices.sql, lists LEFT JOIN clients for `client_name`):
  - `estimates.sql`: `ListEstimates`, `ListEstimatesByStatus`, `ListClientEstimates`, `GetEstimate` (all with the client_name join), `CreateEstimate` (RETURNING *), `UpdateEstimate` (RETURNING *), `UpdateEstimateStatus` (:exec), `DeleteEstimate`, `SetEstimateConverted` (`UPDATE estimates SET converted_invoice_id=?, status='converted', updated_at=? WHERE id=?`).
  - `estimate_line_items.sql`: `ListEstimateLineItems` (WHERE estimate_id=? ORDER BY sort_order, id), `CreateEstimateLineItem`, `DeleteEstimateLineItemsForEstimate`.
  - (Convert reuses the existing invoices/line_items queries — `CreateInvoice`, `CreateLineItem` — to insert the new invoice.)

- [ ] **Step 3: Generate + build.** REPORT exact gen `Estimate`, `EstimateLineItem` structs + Create/Update Params + the join row structs (Estimate cols + ClientName + converted_invoice_id sql.NullInt64). Note nullable mirrors invoices.

- [ ] **Step 4: Migration test** — `TestMigrateCreatesEstimateTables`.

- [ ] **Step 5: Run** db tests, vet, gofmt → version 6. **Commit** `feat(db): estimates + estimate_line_items migration and sqlc queries`.

---

## Task 2: Estimate repository (mirror invoice repo + Convert)

**Files:** Create `internal/repository/estimate.go` (+ `_test.go`). COPY `internal/repository/invoice.go` and adapt.

- [ ] **Step 1: Failing test** — mirror `invoice_test.go`. Cover Create (number EST-0001, totals tax%, snapshots editable-or-default, line items), sequential EST-0001/EST-0002, Get with items, List(+clientName)/ListByStatus/ListClientEstimates, Update, UpdateStatus, Delete (cascade), BulkDelete, BulkUpdateStatus, Duplicate (new number, draft, reset valid_until=""), ClientStats-equivalent NOT needed (estimates have no stats rider), validation (no client / no items → error), 8-goroutine concurrent distinct EST numbers. PLUS **Convert**:
  - Convert an estimate whose status != "accepted" → error (`ErrNotAccepted`).
  - Set status "accepted", Convert → returns {invoiceId, invoiceNumber (INV-...), estimateNumber}; the estimate now has converted_invoice_id set + status "converted"; a real invoice exists (via NewInvoices.Get) with the copied line items + totals + snapshots + due_date == the estimate's valid_until + status "draft".
  - Convert an already-converted estimate → error (`ErrAlreadyConverted`).

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/estimate.go` (adapt invoice.go):
  - Domain `Estimate{...same as Invoice but: EstimateNumber, ValidUntil (no DueDate/PaymentTerms), ConvertedInvoiceID *int64, ... LineItems []*EstimateLineItem}`. `EstimateLineItem` = same as LineItem. `EstimateInput{ClientID int64; Date, ValidUntil string; TaxRate float64; TaxRateID *int64; Notes, Status, CurrencyCode string; BusinessSnapshot, ClientSnapshot, PayerSnapshot string}`. `LineItemInput` — reuse `repository.LineItemInput` (already exists from Batch 3).
  - `EstimatesRepo{db}`, `NewEstimates(db)` panic-if-nil.
  - Create/Get/List/Update/UpdateStatus/Delete/Bulk*/Duplicate — same shape as invoice repo, using `numbering.Estimate` for the number, entity "estimate", snapshots/totals identical. Duplicate resets valid_until="", date=today, status draft, tax_rate_id nil.
  - `var ErrNotAccepted = errors.New("only accepted estimates can be converted")`, `var ErrAlreadyConverted = errors.New("estimate already converted")`.
  - `Convert(ctx, estimateID) (*ConvertResult{InvoiceID int64; InvoiceNumber, EstimateNumber string}, error)`: load estimate (Get); nil → (nil,nil)/notfound; if status != "accepted" → ErrNotAccepted; if ConvertedInvoiceID != nil → ErrAlreadyConverted. Then in `numbering.WithRetry(ctx, 10, func(){ tx:=BeginTx; invNum:=numbering.Next(ctx,tx,numbering.Invoice); insert invoice (client_id, date, due_date=valid_until, subtotal/tax_rate/tax_amount/total copied, notes, status 'draft', currency, snapshots copied, now ts) via gen.CreateInvoice; for each estimate line item insert a gen.CreateLineItem into the new invoice; SetEstimateConverted(estimateID, invoiceId, now); audit.Log(tx, "estimate"/"convert"/estimateID, Changes{invoiceId, invoiceNumber:invNum}); commit; capture invoiceId+invNum })`. Return the result.
  - Reuse the `snapshot`/`round2`/`computeTotals` helpers — to avoid duplication you MAY export the needed helpers from invoice.go (e.g. make `round2` a package-level func already shared) or copy small private ones. Keep funcs <60 lines.

- [ ] **Step 4: Run** `go test ./internal/repository/ -race` + the no-race concurrency `-count=3`. vet, gofmt, build. **Commit** `feat(repository): estimate repository with convert-to-invoice`.

---

## Task 3: Estimate service + handlers + wiring

**Files:** `internal/service/estimate.go`, `internal/http/estimates.go` (+ tests). Modify `server.go`, `cmd/tallyo/main.go`. Mirror invoice service/handlers.

- [ ] **Step 1: Service** — mirror invoice service (entity "estimate"); add `Convert(ctx, id)` → repo.Convert; on success broadcast both `{estimate, id, "convert"}` AND `{invoice, result.InvoiceID, "create"}` (so both lists refresh). No ClientStats.

- [ ] **Step 2: Handlers** (behind RequireAuth) — mirror invoice handlers: `GET /api/estimates` (?status, ?clientId), `POST /api/estimates`, `GET/PUT/DELETE /api/estimates/{id}`, `POST /api/estimates/{id}/status`, `POST /api/estimates/{id}/duplicate`, `POST /api/estimates/{id}/convert` (200 + {invoiceId,invoiceNumber,estimateNumber}; map ErrNotAccepted/ErrAlreadyConverted → 409), `POST /api/estimates/bulk-delete`, `POST /api/estimates/bulk-status`. NO read-time overdue sweep (estimates don't go overdue; they expire via valid_until but the old app has no auto-expire sweep — skip). Tests mirror invoice handler tests + a convert test (accepted→convert→409 on re-convert).

- [ ] **Step 3: Wire** server.go (Deps.Estimates + guard + routes) + cmd/tallyo (construct service+handler into Deps). Nil-safe. (No ticker needed for estimates.)

- [ ] **Step 4: Run** `go test ./... -race`, vet, gofmt; boot smoke (create estimate EST-0001 total 27.5; set status accepted; convert → invoice INV-000x exists, estimate converted; re-convert → 409). **Commit** `feat(estimates): service, REST endpoints, and convert-to-invoice`.

---

## Task 4: Frontend — estimates UI

**Files:** store `web/src/lib/stores/estimates.svelte.ts`; route `web/src/routes/estimates/+page.svelte`; types; nav. COPY the invoices page + adapt (validUntil instead of dueDate; statuses draft/accepted/declined/converted; a "Convert to invoice" action on accepted estimates; no overdue).

- [ ] **Step 1: Types + store** — Estimate, EstimateLineItem, EstimateInput (camelCase, validUntil, convertedInvoiceId). `createCollectionStore<Estimate, EstimateCreatePayload>('estimates','estimate')`.
- [ ] **Step 2: Route** — copy invoices page; rename dueDate→validUntil; status set {draft,accepted,declined,converted}; add a "Convert to Invoice" button shown when status=='accepted' && !convertedInvoiceId → `apiPost('/api/estimates/'+id+'/convert',{})` then reload (and optionally toast the new invoice number); same line-items editor + live totals (tax %); client/tax selects; duplicate; delete; status actions.
- [ ] **Step 3: Nav** — add "Estimates" link (after Invoices).
- [ ] **Step 4: Verify** `npm run check` (0/0), build (200.html), `touch build/.gitkeep`. **Commit** `feat(web): estimates UI with convert-to-invoice`.

---

## Task 5: Batch 4 acceptance

- [ ] **Step 1: Gates** — `go test ./... -race`, vet, gofmt, `npm run check` + build.
- [ ] **Step 2: Live smoke** — boot (wait ready); setup+login; seed client+tax; create estimate (EST-0001, total 27.5, line items, snapshots); status→accepted; convert → assert an invoice INV-000x exists with copied items/totals, estimate.convertedInvoiceId set + status converted; re-convert → 409; duplicate; SSE `estimate`/create event. Capture output.
- [ ] **Step 3: Commit** `chore: batch 4 acceptance — estimates full-stack with convert-to-invoice`.

---

## Done When

- estimates + estimate_line_items migrated; full CRUD + status + bulk + duplicate + numbering (EST-) + snapshots + totals (tax %) mirror invoices; **Convert** turns an accepted estimate into a real invoice (new INV number, copied items/totals/snapshots, estimate marked converted) atomically; guards (not-accepted / already-converted → 409); mutations audited + broadcast.
- Frontend estimates list + line-item editor + convert action.
- All gates green; live smoke confirms numbering, totals, convert→invoice, re-convert guard, SSE.

Batch 5 (payments) links payments to invoices and extends ClientStats with total_paid.
