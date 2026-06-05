# Domain Port — Batch 5: payments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port payments — recorded against invoices — with a derived paid/balance rollup, and extend the client-stats rider with `total_paid`.

**Architecture:** Standard per-domain pattern. Payments belong to an invoice (`invoice_id` FK CASCADE). Recording a payment is a simple audited insert (the old app does NOT auto-transition the invoice to "paid" — balance is **derived**: `balance = invoice.total - totalPaid`). Reads expose `totalPaid` + `payments[]` on the invoice and `total_paid` in client stats.

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md`

**Reference templates:** `internal/repository/{rate_tier,invoice}.go`, `internal/service/invoice.go`, `internal/http/invoices.go` (the `/clients/{id}/stats` rider + sub-routes pattern), `web/src/routes/invoices/+page.svelte`.

**Schema (verbatim, clean-break):**
```sql
payments: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE,
  invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  amount REAL NOT NULL, payment_date TEXT NOT NULL, method TEXT DEFAULT '', notes TEXT DEFAULT '',
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
  -- index: idx_payments_invoice_id(invoice_id)
```

**Parity note:** old `createPayment` inserts only (no status change). `getInvoiceTotalPaid` = `SUM(amount)`. Keep that — do NOT auto-mark invoices paid.

---

## Task 1: Migration 00007 + sqlc queries

**Files:** `internal/db/migrations/00007_payments.sql`; `internal/db/queries/payments.sql`; **modify** `internal/db/queries/invoices.sql` (extend client stats with total_paid); regen; migration test.

- [ ] **Step 1: Migration** — payments table + index. Down drops payments.

- [ ] **Step 2: sqlc.**
  - `payments.sql`: `ListInvoicePayments` (WHERE invoice_id=? ORDER BY payment_date, id), `InvoiceTotalPaid` (`SELECT CAST(COALESCE(SUM(amount),0) AS REAL) AS total_paid FROM payments WHERE invoice_id = ?`), `CreatePayment` (RETURNING *), `GetPayment` (by id — needed to find the invoice for the broadcast/audit on delete), `DeletePayment` (:exec).
  - **Modify** `invoices.sql` `ClientInvoiceStats`: add `total_paid` —
    ```sql
    -- name: ClientInvoiceStats :one
    SELECT COUNT(*) AS invoice_count,
           CAST(COALESCE(SUM(i.total),0) AS REAL) AS total_invoiced,
           CAST(COALESCE((SELECT SUM(p.amount) FROM payments p JOIN invoices i2 ON p.invoice_id=i2.id WHERE i2.client_id = ?1),0) AS REAL) AS total_paid
    FROM invoices i WHERE i.client_id = ?1;
    ```
    (Use the `?1` named param form sqlc supports for the repeated client_id, OR restructure to two params — verify sqlc accepts it; if not, pass clientID twice via a 2-field Params struct.)

- [ ] **Step 3: Generate + build.** REPORT exact `Payment` struct + `CreatePaymentParams` + the updated `ClientInvoiceStatsRow` (now InvoiceCount, TotalInvoiced, TotalPaid float64) + `InvoiceTotalPaidRow`/return type + `GetPayment` return. (amount → float64 NOT NULL; method/notes → sql.NullString.)

- [ ] **Step 4: Migration test** — `TestMigrateCreatesPaymentsTable`.

- [ ] **Step 5: Run** db tests, vet, gofmt → version 7. NOTE: the invoice repo's `ClientStats` mapping must be updated for the new TotalPaid column (Task 2). **Commit** `feat(db): payments migration, queries, and client total_paid stats`.

---

## Task 2: Payment repository + invoice rollup extensions

**Files:** Create `internal/repository/payment.go` (+ `_test.go`); **modify** `internal/repository/invoice.go` (ClientStats gains TotalPaid; add `TotalPaid(ctx, invoiceID)` + include payments/totalPaid on Get).

- [ ] **Step 1: Failing test** (`payment_test.go`, seed a client + invoice via the repos):
  - `Create(ctx, PaymentInput{InvoiceID, Amount, PaymentDate, Method, Notes})` → *Payment, audited ("payment"/"create"). Validate InvoiceID!=0 and Amount>0 (error otherwise).
  - `ListForInvoice(ctx, invoiceID)` → ordered payments (non-nil).
  - `TotalPaid(ctx, invoiceID)` → SUM.
  - `Delete(ctx, id)` (audited "payment"/"delete"; look up the payment first to get invoice_id for the audit/broadcast).
  - Invoice `ClientStats` now returns TotalPaid (record 2 invoices for a client, pay part of one → totalInvoiced = Σ totals, totalPaid = Σ payments).
  - Invoice `Get` includes `TotalPaid` (and optionally `Payments []*Payment`) — assert `Get(invoiceId).TotalPaid` reflects recorded payments and a derived `Balance = Total - TotalPaid`.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement.**
  - `internal/repository/payment.go`: domain `Payment{ID int64; UUID string; InvoiceID int64; Amount float64; PaymentDate, Method, Notes, CreatedAt, UpdatedAt string}` (json camelCase: invoiceId, paymentDate, createdAt, updatedAt). `PaymentInput{InvoiceID int64; Amount float64; PaymentDate, Method, Notes string}`. `PaymentsRepo{db}`, `NewPayments(db)` panic-if-nil. `Create` (validate; uuid; RFC3339; audit "payment"/"create" real id), `ListForInvoice` (non-nil), `TotalPaid(ctx, invoiceID) (float64,error)`, `Delete` (GetPayment for invoice_id; audit "payment"/"delete").
  - `internal/repository/invoice.go`: add `TotalPaid float64 \`json:"totalPaid"\`` and `Balance float64 \`json:"balance"\`` to the `Invoice` domain struct. In `Get`, after loading, compute TotalPaid via the payments query (or a small join) and Balance = round2(Total - TotalPaid). Update the `ClientStats` struct + mapping to include `TotalPaid float64 \`json:"totalPaid"\``. (Lists don't need totalPaid — only Get + ClientStats.)
  - Keep funcs <60 lines.

- [ ] **Step 4: Run** repository tests + full `go test ./internal/repository/ -race`, vet, gofmt, build. **Commit** `feat(repository): payment repository and invoice paid/balance rollup`.

---

## Task 3: Payment service + handlers + wiring

**Files:** Create `internal/service/payment.go`, `internal/http/payments.go` (+ tests). Modify `server.go`, `cmd/tallyo/main.go`.

- [ ] **Step 1: Service** — `PaymentService{repo, hub}` (also needs invoice id for broadcast). Methods: `ListForInvoice(ctx, invoiceID)`, `Create(ctx, PaymentInput)` → broadcast `{payment, id, "create"}` AND `{invoice, invoiceID, "update"}` (so the invoice's balance refreshes in the UI). `Delete(ctx, id)` → look up invoice id (repo.Delete returns it or the service fetches first) → broadcast `{payment, id, "delete"}` + `{invoice, invoiceID, "update"}`. Test: create broadcasts payment+invoice.

- [ ] **Step 2: Handlers** (behind RequireAuth):
  - `GET /api/invoices/{id}/payments` → list for that invoice.
  - `POST /api/invoices/{id}/payments {amount, paymentDate, method, notes}` → 201 + payment (invoiceId from the path).
  - `DELETE /api/payments/{id}` → 204.
  - Validate amount>0 → 400; bad id → 400. Tests (seed invoice): record a payment, list shows it, invoice Get shows totalPaid/balance, delete.

- [ ] **Step 3: Wire** server.go (`Deps.Payments *PaymentHandler` + guard + the 3 routes; the `/invoices/{id}/payments` routes go in the RequireAuth group) + cmd/tallyo (construct service+handler). Nil-safe.

- [ ] **Step 4: Run** `go test ./... -race`, vet, gofmt; boot smoke (create client+invoice total 27.5; POST a payment of 10 → 201; GET invoice → totalPaid 10, balance 17.5; GET payments list; client stats totalPaid 10; delete payment → balance back to 27.5). **Commit** `feat(payments): service, REST endpoints, invoice balance`.

---

## Task 4: Frontend — payments UI

**Files:** modify `web/src/routes/invoices/+page.svelte` (add a per-invoice payments panel) OR add a small payments section; types; (no new nav item — payments live under invoices).

- [ ] **Step 1: Types** — `Payment`, `PaymentInput` (camelCase). Add `totalPaid`/`balance` to the `Invoice` type.
- [ ] **Step 2: UI** — on the invoices page, for each invoice row (or in the edit/detail view), show `total`, `totalPaid`, `balance` (balance = total - totalPaid). Add a "Record Payment" control: a small form (amount, paymentDate default today, method, notes) → `apiPost('/api/invoices/'+id+'/payments', {...})` then reload the invoice list; show the payment list for that invoice (`apiGet('/api/invoices/'+id+'/payments')`) with a delete (X) per payment → `apiDel('/api/payments/'+pid)`. Use `apiGet/apiPost/apiDelete` from `$lib/api/client`. A "Paid"/"Partial"/"Unpaid" badge derived from balance (balance<=0 → Paid green; 0<balance<total → Partial amber; balance==total → Unpaid gray). try/catch.
- [ ] **Step 3: Verify** `npm run check` (0/0), build (200.html), `touch build/.gitkeep`. **Commit** `feat(web): record payments and show invoice balance`.

---

## Task 5: Batch 5 acceptance

- [ ] **Step 1: Gates** — `go test ./... -race`, vet, gofmt, `npm run check` + build.
- [ ] **Step 2: Live smoke** — boot (wait ready); setup+login; seed client+tax; create invoice (total 27.5); POST payment 10 → invoice totalPaid 10, balance 17.5; POST payment 17.5 → balance 0 (Paid); GET /api/clients/{id}/stats → totalPaid reflects; DELETE a payment → balance increases; SSE `payment`/create + `invoice`/update events. Capture output.
- [ ] **Step 3: Commit** `chore: batch 5 acceptance — payments full-stack`.

---

## Done When

- payments migrated; record/list/delete payments per invoice; invoice Get exposes `totalPaid`+`balance` (derived, no auto-paid transition — parity); client stats include `total_paid`; mutations audited + broadcast (payment + invoice events).
- Frontend records payments + shows balance/paid badge per invoice.
- All gates green; live smoke confirms balance math, client stats total_paid, SSE.

Batch 6 (recurring) generates invoices from templates on a schedule.
