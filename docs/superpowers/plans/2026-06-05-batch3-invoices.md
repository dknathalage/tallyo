# Domain Port — Batch 3: invoices Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port invoices + line_items full-stack: server-computed numbering (the Batch-0 atomicity contract), business/client/payer snapshots, totals, status lifecycle (incl. an overdue sweep), bulk ops, duplicate, and the deferred client-stats rider.

**Architecture:** Per-domain pattern + invoice-specific logic. Invoice Create runs in ONE transaction wrapped in `numbering.WithRetry`: generate `invoice_number` via `numbering.Next(tx, numbering.Invoice)`, capture point-in-time snapshots, compute totals server-side, insert the invoice + its line_items, audit. Reads return the invoice WITH its line items. An overdue sweep (`sent`→`overdue` when `due_date < today`) runs on launch + via an in-session ticker.

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md`

**Reference templates:** Batch 1/2 repos/services/handlers; `internal/numbering` (Next + WithRetry + Invoice config); `internal/repository/{client,catalog,tax_rate}.go`; `internal/repository/business_profile.go` (Get for snapshot source).

**Schema to port (verbatim, clean-break):**
```sql
invoices: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, invoice_number TEXT NOT NULL UNIQUE,
  client_id INTEGER NOT NULL REFERENCES clients(id), date TEXT NOT NULL, due_date TEXT NOT NULL,
  payment_terms TEXT DEFAULT 'custom', subtotal REAL DEFAULT 0, tax_rate REAL DEFAULT 0,
  tax_rate_id INTEGER REFERENCES tax_rates(id) ON DELETE SET NULL, tax_amount REAL DEFAULT 0,
  total REAL DEFAULT 0, notes TEXT DEFAULT '', status TEXT DEFAULT 'draft',
  currency_code TEXT DEFAULT 'USD', business_snapshot TEXT DEFAULT '{}',
  client_snapshot TEXT DEFAULT '{}', payer_snapshot TEXT DEFAULT '{}',
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
  -- indexes: idx_invoices_status(status), idx_invoices_client_id(client_id), idx_invoices_created_at(created_at)
line_items: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  description TEXT NOT NULL, quantity REAL NOT NULL DEFAULT 1, rate REAL NOT NULL DEFAULT 0,
  amount REAL NOT NULL DEFAULT 0, notes TEXT DEFAULT '', sort_order INTEGER DEFAULT 0,
  catalog_item_id INTEGER, rate_tier_id INTEGER
  -- index: idx_line_items_invoice_id(invoice_id)
```

**Server-computed Create/Update semantics (port of the old route + createInvoice):**
- Each line item `amount = quantity * rate` (computed server-side, never trust the client).
- `subtotal = Σ amounts`.
- `tax_rate`: if `taxRateId` set, look up that tax rate's `rate`; else 0. `tax_amount = subtotal * tax_rate`. `total = subtotal + tax_amount`.
- `invoice_number`: `numbering.Next(ctx, tx, numbering.Invoice)` inside the create tx; the whole create wrapped in `numbering.WithRetry`.
- Snapshots (JSON, point-in-time): `business_snapshot` = current business_profile {name,email,phone,address,logo,metadata}; `client_snapshot` = client {name,email,phone,address,metadata}; `payer_snapshot` = the client's payer {name,email,phone,address,metadata} if `payer_id` set, else `{}`.
- `status` default `draft`.

---

## Task 1: Migration 00005 + sqlc queries (invoices, line_items)

**Files:** Create `internal/db/migrations/00005_invoices.sql`; `internal/db/queries/{invoices,line_items}.sql`; regen gen; migration test.

- [ ] **Step 1: Migration** — both tables verbatim (FK client_id→clients NOT NULL; tax_rate_id→tax_rates SET NULL; line_items.invoice_id→invoices CASCADE; the 3 invoice indexes + line_items index). Down drops line_items then invoices.

- [ ] **Step 2: sqlc queries.**
  - `invoices.sql`: `ListInvoices` (ORDER BY created_at DESC), `ListInvoicesByStatus` (WHERE status=? ORDER BY created_at DESC), `SearchInvoices` (WHERE invoice_number LIKE ? ORDER BY created_at DESC — and/or join client name; keep to invoice_number + a client_id filter for now), `GetInvoice` (by id), `ListClientInvoices` (WHERE client_id=? ORDER BY created_at DESC), `CreateInvoice` (RETURNING *), `UpdateInvoice` (RETURNING *), `DeleteInvoice`, `UpdateInvoiceStatus` (SET status=?, updated_at=? WHERE id=?), `MaxInvoiceNumber`-not-needed (numbering pkg does it), `SelectOverdueInvoices` (`SELECT id, invoice_number FROM invoices WHERE status='sent' AND due_date < date('now')`), `MarkInvoicesOverdue` (`UPDATE invoices SET status='overdue', updated_at=? WHERE status='sent' AND due_date < date('now')`). Client-stats rider: `ClientInvoiceStats` (`SELECT COUNT(*) AS invoice_count, COALESCE(SUM(total),0) AS total_invoiced FROM invoices WHERE client_id=?` — total_paid comes in Batch 5; for now total_invoiced + count).
  - `line_items.sql`: `ListLineItems` (WHERE invoice_id=? ORDER BY sort_order, id), `CreateLineItem`, `DeleteLineItemsForInvoice` (WHERE invoice_id=?).

- [ ] **Step 3: Generate + build.** REPORT exact gen `Invoice`, `LineItem` structs + Create/Update Params (note: subtotal/tax_rate/tax_amount/total → float64 likely `sql.NullFloat64` due to DEFAULT 0; status/notes/etc → sql.NullString; tax_rate_id/catalog_item_id/rate_tier_id → sql.NullInt64). Report the overdue/stats row types.

- [ ] **Step 4: Migration test** — `TestMigrateCreatesInvoiceTables` (invoices, line_items).

- [ ] **Step 5: Run** db tests, vet, gofmt → version 5. **Commit** `feat(db): invoices + line_items migration and sqlc queries`.

---

## Task 2: Invoice repository (numbering + snapshots + totals + line items)

**Files:** Create `internal/repository/invoice.go` (+ `_test.go`). This is the largest repo — split helpers; keep each func <60 lines.

- [ ] **Step 1: Failing test** (temp migrated DB; seed a business_profile, a client with a payer + metadata, a tax rate). Cover:
  - `Create(ctx, InvoiceInput, []LineItemInput)` → returns *Invoice with a generated `invoiceNumber` matching `INV-\d{4}`, status "draft", computed subtotal/taxAmount/total (e.g. 2 items 2×10 + 1×5 = 25 subtotal; taxRate 0.1 → taxAmount 2.5, total 27.5), and snapshots populated (clientSnapshot has the client name; payerSnapshot has the payer name; businessSnapshot has the business name). Line items persisted with amount=qty*rate.
  - Sequential creates → INV-0001, INV-0002.
  - `Get(ctx, id)` returns the invoice WITH its line items (ordered by sort_order).
  - `List` newest-first; `ListByStatus("draft")`; `ListClientInvoices(clientId)`.
  - `Update(ctx, id, input, items)` → replaces line items (delete-then-insert) and recomputes totals; updates updated_at.
  - `UpdateStatus(ctx, id, "sent")`. `Delete(ctx, id)` (line items cascade). `BulkDelete`. `BulkUpdateStatus(ids, "sent")`.
  - `Duplicate(ctx, id)` → new invoice, new number, status "draft", copied line items + fields (fresh snapshots OR copied — match old `duplicateInvoice`; copying the snapshots/fields with a new number+draft status is fine).
  - `MarkOverdue(ctx)` → invoices with status "sent" and past due_date become "overdue"; returns the affected {id, number}. (Set one invoice sent + due_date yesterday; assert it flips.)
  - `ClientStats(ctx, clientId)` → {invoiceCount, totalInvoiced}.
  - Create validation: empty client_id (0) or no line items → error.
  - **Concurrency:** a parallel-create test (8 goroutines) asserts distinct invoice numbers (proves the numbering+WithRetry integration). Use the `numbering` pattern from `internal/numbering/numbering_test.go`.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/invoice.go`:
  - Domain `Invoice{ID int64; UUID, InvoiceNumber string; ClientID int64; Date, DueDate, PaymentTerms string; Subtotal, TaxRate float64; TaxRateID *int64; TaxAmount, Total float64; Notes, Status, CurrencyCode string; BusinessSnapshot, ClientSnapshot, PayerSnapshot string; CreatedAt, UpdatedAt string; LineItems []*LineItem}` (camelCase json: invoiceNumber, clientId, dueDate, paymentTerms, taxRate, taxRateId, taxAmount, currencyCode, businessSnapshot, clientSnapshot, payerSnapshot, lineItems, etc.).
  - `LineItem{ID int64; UUID string; Description string; Quantity, Rate, Amount float64; Notes string; SortOrder int64; CatalogItemID *int64; RateTierID *int64}`.
  - `InvoiceInput{ClientID int64; Date, DueDate, PaymentTerms string; TaxRateID *int64; Notes, Status, CurrencyCode string}`. `LineItemInput{Description string; Quantity, Rate float64; Notes string; SortOrder int64; CatalogItemID *int64; RateTierID *int64}`.
  - `InvoicesRepo{db}`, `NewInvoices(db)` panic-if-nil.
  - `Create(ctx, in, items)`: validate in.ClientID != 0 and len(items) > 0. Resolve `taxRate` (if in.TaxRateID != nil, SELECT rate FROM tax_rates WHERE id — via gen GetTaxRate or a direct query; on missing → 0). Build snapshots: read business_profile (gen GetBusinessProfile), client (gen GetClient), and the client's payer (gen GetPayer if client.PayerID valid) → JSON via a `snapshot()` helper {name,email,phone,address,metadata}. Compute amounts/subtotal/tax_amount/total. Then `numbering.WithRetry(ctx, 10, func() error { tx := BeginTx; defer rollback; num := numbering.Next(ctx, tx, numbering.Invoice); insert invoice (num, computed totals, snapshots, status default draft, now ts); for each item insert line_item (amount=qty*rate, uuid, sort_order); audit.Log(tx, "invoice"/"create"/newID); commit })`. Capture the new id, then return `Get(ctx, id)`.
  - `Get(ctx, id)`: load invoice row (nil-missing) + ListLineItems; map. `List`/`ListByStatus`/`ListClientInvoices`: load rows (NO line items in list — keep lists light; line items only on Get). Non-nil slices.
  - `Update(ctx, id, in, items)`: recompute totals + tax; in a tx: UpdateInvoice, DeleteLineItemsForInvoice, re-insert items, audit "update". (Snapshots NOT regenerated on update — keep the original point-in-time snapshots; match old behavior which preserves them.) Return Get. nil-missing.
  - `UpdateStatus(ctx, id, status)`: audit "status". `Delete`, `BulkDelete`, `BulkUpdateStatus`.
  - `Duplicate(ctx, id)`: load source; create a new invoice (new number via numbering, status "draft", copy date/due/terms/tax/notes/currency/snapshots + line items). Return new *Invoice.
  - `MarkOverdue(ctx) ([]OverdueInvoice, error)`: SELECT overdue, UPDATE; return affected {ID, InvoiceNumber}. (Used by the sweep.)
  - `ClientStats(ctx, clientID) (*ClientStats, error)`: {InvoiceCount int64; TotalInvoiced float64}.
  - Helpers: `snapshot()` builder, `nullID`/`ptrID`, float null wrap. Keep funcs <60 lines (extract `computeTotals`, `buildSnapshots`, `insertLineItems`).

- [ ] **Step 4: Run** `go test ./internal/repository/ -race` (incl the 8-goroutine distinct-number test; run `-count=5` without -race to confirm numbering stability). vet, gofmt, build. **Commit** `feat(repository): invoice repository with numbering, snapshots, totals, line items`.

---

## Task 3: Invoice service + HTTP handlers + wiring

**Files:** Create `internal/service/invoice.go`, `internal/http/invoices.go` (+ tests). Modify `internal/http/server.go`, `cmd/tallyo/main.go`.

- [ ] **Step 1: Service** (`internal/service/invoice.go`) — holds repo + hub; broadcast `invoice`/<id>/<action> after success (create/update/delete/status/duplicate; bulk → id 0). Methods mirror the repo (List/ListByStatus/Get/Create/Update/UpdateStatus/Delete/BulkDelete/BulkUpdateStatus/Duplicate/MarkOverdue/ClientStats/ListClientInvoices). MarkOverdue broadcasts an `invoice`/0/`overdue_sweep` event if any flipped. Tests: create broadcasts; status change broadcasts.

- [ ] **Step 2: Handlers** (`internal/http/invoices.go`, behind RequireAuth):
  - `GET /api/invoices` (optional `?status=` filter, `?clientId=` filter), `POST /api/invoices` (body `{...invoiceInput, lineItems:[...]}` → 201 + full invoice), `GET /api/invoices/{id}` (200 with line items / 404), `PUT /api/invoices/{id}` (update + items), `DELETE /api/invoices/{id}` (204), `POST /api/invoices/{id}/status {status}` (200), `POST /api/invoices/{id}/duplicate` (201 + new), `POST /api/invoices/bulk-delete {ids}`, `POST /api/invoices/bulk-status {ids,status}`, `GET /api/clients/{id}/stats` (the client-stats rider — register under clients or invoices; 200 {invoiceCount,totalInvoiced}).
  - Validate: client id present, ≥1 line item → 400; bad id → 400; nil → 404.
  - Tests (local-router + owner + seeded client/tax): create computes totals + number + snapshots; get returns line items; status flip; duplicate; bulk; overdue not directly HTTP-tested (it's a sweep) but ClientStats endpoint tested.

- [ ] **Step 3: Wire server.go** — `Deps.Invoices *InvoiceHandler` + guard + routes (including the `/clients/{id}/stats` route — note it lives under the clients path but is served by the invoice handler; register it in the same group). Nil-safe.

- [ ] **Step 4: Overdue sweep in cmd/tallyo** — after constructing the invoice service, run `invoiceSvc.MarkOverdue(ctx)` once at startup (log the count), and start a bounded `time.Ticker` (e.g. hourly) in a goroutine that calls MarkOverdue, stopped on shutdown (select on a done channel / ctx). Keep it simple + leak-free (ticker.Stop on shutdown).

- [ ] **Step 5: Run** `go test ./... -race`, vet, gofmt; boot smoke (create an invoice via curl with a seeded client+tax, verify number/totals/snapshots/line-items; flip status; duplicate). **Commit** `feat(invoices): service, REST endpoints, and overdue sweep`.

---

## Task 4: Frontend — invoices UI

**Files:** store `web/src/lib/stores/invoices.svelte.ts`; route `web/src/routes/invoices/+page.svelte` (+ maybe a detail/new view); types; nav.

- [ ] **Step 1: Types + store** — `Invoice`, `LineItem`, `InvoiceInput`, `LineItemInput` (camelCase). `createCollectionStore<Invoice, ...>('invoices','invoice')` for the list (note: create/update need the lineItems payload, which the generic crud.create(input) handles if input includes lineItems — define the create payload type as `{...invoiceInput, lineItems:[...]}`).
- [ ] **Step 2: Route** — list with status filter (badges per status) + create/edit form: client `<select>` (from clients store), tax-rate `<select>` (from taxRates store), date/dueDate, a dynamic line-items editor (add/remove rows: description, quantity, rate; show computed amount + live subtotal/tax/total preview), status actions (Mark sent / etc. via `/status`), duplicate button, delete. Load clients + taxRates stores for the selects. Show server-computed totals after save. Client-side search filter.
- [ ] **Step 3: Nav** — add "Invoices" link (make it prominent / first).
- [ ] **Step 4: Verify** `npm run check` (0/0), `npm run build` (200.html), `touch build/.gitkeep`. **Commit** `feat(web): invoices UI with line items and status`.

---

## Task 5: Batch 3 acceptance

- [ ] **Step 1: Gates** — `go test ./... -race`, vet, gofmt, `npm run check` + build.
- [ ] **Step 2: Live smoke** — boot (wait for readiness); setup+login; seed client+tax; create invoice (assert number INV-0001, computed total, snapshots, line items); create a 2nd (INV-0002); flip status to sent; create one with due_date in the past + status sent then hit a restart (or call the sweep path) to verify overdue; duplicate; ClientStats endpoint; an SSE `invoice`/create event. Capture output.
- [ ] **Step 3: Commit** `chore: batch 3 acceptance — invoices full-stack`.

---

## Done When

- invoices + line_items migrated; Create generates a unique number (concurrency-safe), captures snapshots, computes totals server-side, persists line items — all atomically; Get returns line items; status lifecycle + overdue sweep (launch + ticker) work; bulk ops + duplicate + client-stats rider present; mutations audited + broadcast.
- Frontend invoice list + line-item editor with client/tax selects, status actions, live totals.
- All gates green; live smoke confirms numbering, totals, snapshots, status, overdue, duplicate, stats, SSE.

Batch 4 (estimates) mirrors this (estimate_number, estimate_line_items, convert-to-invoice writing into this invoices table). Batch 5 (payments) adds total_paid to ClientStats.
