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

**Create/Update semantics (faithful to the old app — see review findings):**
- **Tax is a PERCENTAGE.** `tax_rates.rate` stores e.g. `10` for 10%. `tax_amount = round2(subtotal * (tax_rate / 100))`; `total = subtotal + tax_amount`. (The old form: `taxRate = selectedRate.rate` (the percentage), `taxAmount = round(subtotal*(taxRate/100))`.) Storing a 100×-scaled tax_amount would corrupt every taxed invoice — DO NOT treat the rate as a fraction.
- Totals are **recomputed server-side** from the line items (money is never trusted from the client): per item `amount = quantity * rate`; `subtotal = Σ amounts`; tax/total as above. `tax_rate` (the percentage) comes from the request body; if the body omits it but `taxRateId` is set, look it up from `tax_rates.rate`.
- **Snapshots are EDITABLE and arrive in the request body** (the old form lets the user edit client/payer details + metadata on the invoice). Accept `businessSnapshot`/`clientSnapshot`/`payerSnapshot` as JSON strings in the create/update payload and store them verbatim. **Fallback only:** if a snapshot field is empty/omitted, the server builds a default from the DB — `business_profile`, `GetClient(clientId)`, and the client's payer (`GetPayer`) — each as `{name,email,phone,address,metadata}` (NO `logo` field; matches the old writer's shape). This preserves per-invoice edits while giving a sensible default.
- `invoice_number`: `numbering.Next(ctx, tx, numbering.Invoice)` inside the create tx; the whole create wrapped in `numbering.WithRetry`. Server-authoritative.
- `status` default `draft`.
- `round2(x)` = round to 2 decimals (match the old `Math.round(x*100)/100`).

---

## Task 1: Migration 00005 + sqlc queries (invoices, line_items)

**Files:** Create `internal/db/migrations/00005_invoices.sql`; `internal/db/queries/{invoices,line_items}.sql`; regen gen; migration test.

- [ ] **Step 1: Migration** — both tables verbatim (FK client_id→clients NOT NULL; tax_rate_id→tax_rates SET NULL; line_items.invoice_id→invoices CASCADE; the invoice indexes: `idx_invoices_uuid` UNIQUE on uuid, `idx_invoices_status`, `idx_invoices_client_id`, `idx_invoices_created_at`; + `idx_line_items_invoice_id`). Down drops line_items then invoices.

- [ ] **Step 2: sqlc queries.**
  - `invoices.sql` (**lists LEFT JOIN clients to surface `client_name`** — the old list/get carry it): `ListInvoices` (`SELECT i.*, c.name AS client_name FROM invoices i LEFT JOIN clients c ON i.client_id=c.id ORDER BY i.created_at DESC`), `ListInvoicesByStatus` (+ WHERE i.status=?), `ListClientInvoices` (+ WHERE i.client_id=?), `GetInvoice` (same join, WHERE i.id=?), `CreateInvoice` (RETURNING *), `UpdateInvoice` (RETURNING *), `DeleteInvoice`, `UpdateInvoiceStatus` (SET status=?, updated_at=? WHERE id=?), `SelectOverdueInvoices` (`SELECT id, invoice_number FROM invoices WHERE status='sent' AND due_date < date('now')`), `MarkInvoicesOverdue` (`UPDATE invoices SET status='overdue', updated_at=? WHERE id=?` — applied per-id so each flip can be audited). Client-stats rider: `ClientInvoiceStats` (`SELECT COUNT(*) AS invoice_count, COALESCE(SUM(total),0) AS total_invoiced FROM invoices WHERE client_id=?` — total_paid comes in Batch 5; for now total_invoiced + count). NOTE the join queries generate distinct `*Row` structs (Invoice cols + `ClientName sql.NullString`) — report them.
  - `line_items.sql`: `ListLineItems` (WHERE invoice_id=? ORDER BY sort_order, id), `CreateLineItem`, `DeleteLineItemsForInvoice` (WHERE invoice_id=?).

- [ ] **Step 3: Generate + build.** REPORT exact gen `Invoice`, `LineItem` structs + Create/Update Params (note: subtotal/tax_rate/tax_amount/total → float64 likely `sql.NullFloat64` due to DEFAULT 0; status/notes/etc → sql.NullString; tax_rate_id/catalog_item_id/rate_tier_id → sql.NullInt64). Report the overdue/stats row types.

- [ ] **Step 4: Migration test** — `TestMigrateCreatesInvoiceTables` (invoices, line_items).

- [ ] **Step 5: Run** db tests, vet, gofmt → version 5. **Commit** `feat(db): invoices + line_items migration and sqlc queries`.

---

## Task 2: Invoice repository (numbering + snapshots + totals + line items)

**Files:** Create `internal/repository/invoice.go` (+ `_test.go`). This is the largest repo — split helpers; keep each func <60 lines.

- [ ] **Step 1: Failing test** (temp migrated DB; seed a business_profile, a client with a payer + metadata, a tax rate). Cover:
  - `Create(ctx, InvoiceInput, []LineItemInput)` → returns *Invoice with a generated `invoiceNumber` matching `INV-\d{4}`, status "draft", computed subtotal/taxAmount/total. Worked example: 2 items (2×10) + (1×5) = subtotal 25; **taxRate 10 (percent) → taxAmount 2.5, total 27.5**. Snapshots: (a) when the input carries snapshot JSON, it is stored verbatim; (b) when omitted, the server builds defaults — clientSnapshot has the client name, payerSnapshot has the payer name, businessSnapshot has the business name (each `{name,email,phone,address,metadata}`, no logo). Test BOTH paths. Line items persisted with amount=qty*rate.
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
  - `InvoiceInput{ClientID int64; Date, DueDate, PaymentTerms string; TaxRate float64; TaxRateID *int64; Notes, Status, CurrencyCode string; BusinessSnapshot, ClientSnapshot, PayerSnapshot string}` (json camelCase; TaxRate is the percentage; the three snapshot fields are optional JSON strings — empty → server builds default). `LineItemInput{Description string; Quantity, Rate float64; Notes string; SortOrder int64; CatalogItemID *int64; RateTierID *int64}`.
  - `InvoicesRepo{db}`, `NewInvoices(db)` panic-if-nil.
  - `Create(ctx, in, items)`: validate in.ClientID != 0 and len(items) > 0. Resolve `taxRate` (percentage): use in.TaxRate; if 0 and in.TaxRateID != nil, look up `tax_rates.rate`. Resolve snapshots: for each of business/client/payer, if the input field is non-empty use it verbatim, ELSE build a default via a `snapshot()` helper from the DB (business_profile / GetClient / client's GetPayer) as `{name,email,phone,address,metadata}`. Compute amounts (qty*rate), subtotal (Σ), `tax_amount = round2(subtotal * taxRate/100)`, `total = subtotal + tax_amount`. Then `numbering.WithRetry(ctx, 10, func() error { tx := BeginTx; defer rollback; num := numbering.Next(ctx, tx, numbering.Invoice); insert invoice (num, computed totals, taxRate, taxRateID, snapshots, status default draft, now ts); for each item insert line_item (amount=qty*rate, uuid, sort_order); audit.Log(tx, "invoice"/"create"/newID); commit })`. Capture the new id, then return `Get(ctx, id)`.
  - `Get(ctx, id)`: load invoice row (nil-missing) + ListLineItems; map. `List`/`ListByStatus`/`ListClientInvoices`: load rows (NO line items in list — keep lists light; line items only on Get). Non-nil slices.
  - `Update(ctx, id, in, items)`: recompute totals + tax from the items + in.TaxRate; in a tx: UpdateInvoice (set fields + totals + updated_at), DeleteLineItemsForInvoice, re-insert items, audit "update". Snapshots: if the input provides a non-empty snapshot use it (the form may have edited client/payer details), else preserve the existing stored snapshot (do NOT re-derive from current DB rows — these are point-in-time). Return Get. nil-missing.
  - `UpdateStatus(ctx, id, status)`: audit "status". `Delete`, `BulkDelete`, `BulkUpdateStatus`.
  - `Duplicate(ctx, id)`: load source; create a new invoice (new number via numbering, status "draft") with **old-parity resets: `date = today`, `due_date = ""`, `payment_terms = "custom"`, `tax_rate_id` NOT carried over (omit)**; copy `tax_rate`/`notes`/`currency`/the three snapshots + line items. Return new *Invoice.
  - `MarkOverdue(ctx) ([]OverdueInvoice, error)`: SELECT overdue (status 'sent' AND due_date < date('now')); for EACH flipped invoice UPDATE status='overdue' + write a per-invoice `audit.Log("invoice"/"status"/id, changes {from:"sent",to:"overdue"})` (matches the old per-flip status_change audit); one tx. Return affected {ID, InvoiceNumber}.
  - `ClientStats(ctx, clientID) (*ClientStats, error)`: {InvoiceCount int64; TotalInvoiced float64}.
  - Helpers: `snapshot()` builder, `nullID`/`ptrID`, float null wrap. Keep funcs <60 lines (extract `computeTotals`, `buildSnapshots`, `insertLineItems`).

- [ ] **Step 4: Run** `go test ./internal/repository/ -race` (incl the 8-goroutine distinct-number test; run `-count=5` without -race to confirm numbering stability). vet, gofmt, build. **Commit** `feat(repository): invoice repository with numbering, snapshots, totals, line items`.

---

## Task 3: Invoice service + HTTP handlers + wiring

**Files:** Create `internal/service/invoice.go`, `internal/http/invoices.go` (+ tests). Modify `internal/http/server.go`, `cmd/tallyo/main.go`.

- [ ] **Step 1: Service** (`internal/service/invoice.go`) — holds repo + hub; broadcast `invoice`/<id>/<action> after success (create/update/delete/status/duplicate; bulk → id 0). Methods mirror the repo (List/ListByStatus/Get/Create/Update/UpdateStatus/Delete/BulkDelete/BulkUpdateStatus/Duplicate/MarkOverdue/ClientStats/ListClientInvoices). MarkOverdue broadcasts an `invoice`/0/`overdue_sweep` event if any flipped. Tests: create broadcasts; status change broadcasts.

- [ ] **Step 2: Handlers** (`internal/http/invoices.go`, behind RequireAuth):
  - `GET /api/invoices` (optional `?status=` filter, `?clientId=` filter), `POST /api/invoices` (body `{...invoiceInput, lineItems:[...]}` → 201 + full invoice), `GET /api/invoices/{id}` (200 with line items / 404), `PUT /api/invoices/{id}` (update + items), `DELETE /api/invoices/{id}` (204), `POST /api/invoices/{id}/status {status}` (200), `POST /api/invoices/{id}/duplicate` (201 + new), `POST /api/invoices/bulk-delete {ids}`, `POST /api/invoices/bulk-status {ids,status}`, `GET /api/clients/{id}/stats` (the client-stats rider — register under clients or invoices; 200 {invoiceCount,totalInvoiced}).
  - **Read-time overdue sweep (old parity):** the old app calls `markOverdueInvoices()` on every `GET /api/invoices`. Replicate: the `List` handler calls `svc.MarkOverdue(ctx)` (ignoring its result, logging errors) BEFORE listing, so a freshly-overdue invoice shows correctly without waiting for the ticker. (The launch sweep + ticker remain as a backstop.)
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
