# Tallyo Domain Port — Decomposition Design

**Date:** 2026-06-05
**Status:** Approved (design phase)
**Builds on:** `2026-06-04-go-web-service-rewrite-design.md` (architecture) and the
walking-skeleton-v2 implementation (the proven pattern). This spec decomposes the
remaining feature port into dependency-ordered, full-stack batches.

## Goal

Port every remaining domain from the old Electron/SvelteKit app (`src/`) onto the
Go web-service skeleton, full-stack (Go API + Svelte SPA), reaching feature
parity for: clients, payers, tax-rates, rate-tiers, catalog, invoices, estimates,
payments, recurring, PDF generation, and import/export.

## Nature of the work

This is a **parity port, not new design.** The old app is the exact behavioral
reference; the architecture is settled by the skeleton. Therefore:

- No per-domain brainstorm. ONE decomposition spec (this), then ONE implementation
  plan per batch, each executed via subagent-driven TDD with two-stage review.
- The BusinessProfile slice is the copy-template for every domain.

## Per-domain pattern (applied to each)

1. **Migration** — a goose migration creating the domain's tables, ported
   verbatim (column names/types/constraints/indexes) from
   `src/lib/db/drizzle-schema.ts`. Clean-break (fresh schema, no data migration).
2. **sqlc queries** — `internal/db/queries/<domain>.sql`; regenerate `gen`.
3. **Repository** — `internal/repository/<domain>.go`: domain types (plain,
   unwrap `sql.NullString`), audited mutations via the Batch-0 `WithAudit`
   tx-wrapper. Reference: the old `src/lib/db/queries/<domain>.ts` for behavior.
4. **Service** — `internal/service/<domain>.go`: orchestration + SSE
   `hub.Broadcast` after commit.
5. **HTTP handlers** — `internal/http/<domain>.go`: REST under `/api/<domain>`,
   behind `RequireAuth`, passing `r.Context()`. JSON camelCase (matches Go json
   tags).
6. **Frontend** — `web/src/lib/stores/<domain>.svelte.ts` (rune store via the
   Batch-0 factory, SSE-wired), routes under `web/src/routes/<domain>/`, components
   ported from `src/lib/components/<domain>/`.
7. **Tests** — Go: repository (temp modernc DB) + handler (`httptest`) tests;
   frontend: `svelte-check` clean, vitest where logic warrants.

Each batch ends with acceptance: `go test ./... -race`, `go vet`, `gofmt`,
`npm run build` + `npm run check`, and a live curl/boot smoke of the new endpoints.

## Foundation refactors (Batch 0 — do FIRST)

These get replicated 13×, so they land before any domain (carry-forward items
from the skeleton-v2 review):

1. **`repository.WithAudit(ctx, db, entry, func(tx) error) error`** — a tx-wrapper
   enforcing BeginTx → fn(tx) → audit.Log(tx) → Commit (rollback on any error).
   Refactor the existing BusinessProfile/users/invites repos onto it. Makes the
   audit-on-mutation invariant structural, not per-repo discipline.
2. **Standardized audit `Entry`** — real `entity_id` and a before/after `changes`
   convention (not hardcoded `ID:1` / `{"name":...}`). Define a small helper to
   build the changes JSON; apply going forward.
3. **Invite `Accept` in a single transaction** — create user + mark invite used
   atomically; map duplicate-email to **409** (not 500).
4. **Embed startup self-check** — fail/log clearly if the embedded SPA has no
   `200.html` (clean-clone/CI guard). Document `npm run build` before `go build`.
5. **Drop the SSE `Connection: keep-alive` header** (hop-by-hop, invalid HTTP/2).
6. **`internal/numbering`** — invoice/estimate sequence generation, ported from
   `src/lib/db/number-generators.ts` + `src/lib/utils/{invoice,estimate}-number.ts`.
   **CRITICAL atomicity contract:** the old `SELECT MAX(...) then INSERT` is
   non-atomic and only safe under single-threaded WASM; the Go server is
   concurrent. The number must be generated **inside the same transaction** as the
   invoice/estimate insert, with **retry-on-unique-conflict** (the
   `invoice_number`/`estimate_number` UNIQUE constraint is the backstop). The
   numbering API must therefore accept a `*sql.Tx` (or run within the document's
   create tx), NOT a standalone call. This is the #1 thing that compounds across
   Batches 3, 4, 6 — get it right in Batch 0.
7. **`internal/money`** — currency/formatting helpers ported from
   `src/lib/utils/currency.ts` if domains need them.
8. **Shared frontend infra** — a generic CRUD API helper (list/get/create/update/
   delete over `/api/<domain>`), a domain-store factory (rune `$state` collection
   + SSE `onEntity` refetch), and reusable list/form components, so each domain
   UI is thin.

## Batch order (each full-stack)

| Batch | Domains | Depends on |
|-------|---------|-----------|
| 0 | Foundation refactors + numbering + money + shared FE infra | skeleton |
| 1 | rate_tiers, payers | 0 |
| 2 | tax_rates, clients, catalog (+catalog_item_rates) | 0, 1 |
| 3 | invoices (+line_items, numbering, status, snapshots, overdue sweep, client-stats rider) | 1, 2 |
| 4 | estimates (+estimate_line_items, numbering, convert-to-invoice) | 1, 2, 3 |
| 5 | payments (linked to invoices, paid/AR rollup) | 3 |
| 6 | recurring-templates (+idempotent sweep + ticker) | 3, 4 |
| 7 | PDF generation (maroto) for invoices + estimates | 3, 4 |
| 8 | import/export (export invoices+estimates+catalog; import catalog CSV+Excel + column_mappings) | 2, 3, 4 |

**Why this order (FKs confirmed from the schema + old query modules):**
- `clients → rate_tiers (pricing_tier_id, SET NULL), payers` — so **rate_tiers and
  payers must precede clients** (clients moved to Batch 2, after Batch 1 builds
  rate_tiers + payers). `clients.ts` also left-joins rate_tiers for
  `pricing_tier_name`.
- `invoices → clients, tax_rates`; `line_items → invoices(cascade), catalog_items,
  rate_tiers`; `estimates → clients, tax_rates`;
  `estimate_line_items → estimates(cascade)`; `payments → invoices(cascade)`;
  `catalog_item_rates → catalog_items, rate_tiers`.
- **Back-dependencies the topological order can't satisfy (handled as forward-ref
  riders, recorded so the order stays honest):**
  - **Client stats** (`getClientStats`: total_invoiced/total_paid/outstanding/
    invoice_count) aggregate over the invoices table. Ship clients in Batch 2
    WITHOUT stats; add the stats endpoint as a **rider on Batch 3** once invoices
    exist.
  - **Estimate→invoice conversion** (`convertEstimateToInvoice`) writes into
    invoices+line_items and sets `estimates.converted_invoice_id` → Batch 4
    depends on Batch 3. Port conversion + duplicate as estimate behaviors.

Invoices/estimates also store `business_snapshot`/`client_snapshot`/
`payer_snapshot` JSON (denormalized point-in-time copies) — port that behavior.

**Automated lifecycle sweeps (concurrent server, no always-on Node):**
- **markOverdueInvoices** (`sent → overdue` when `due_date < now`) — a boot +
  in-session ticker job. Fold into **Batch 3** (invoice status), co-located with
  the Batch-6 scheduling infra pattern.
- **recurring generation** — see Batch 6 below.

## Tables to migrate (from drizzle-schema.ts, clean-break)

clients, payers, tax_rates, rate_tiers, catalog_items, catalog_item_rates,
invoices, line_items, estimates, estimate_line_items, payments,
recurring_templates, column_mappings. (audit_log, business_profile, users,
invites, sessions already exist. ai_chat_* are NOT recreated.)

Each batch adds the goose migration(s) for its tables, numbered sequentially after
`00002_auth.sql`.

## Scheduling (Batch 6)

The Go server only runs while up (no always-on Node server). Recurring generation:
run-on-launch sweep (generate due invoices at boot) + a bounded in-session
`time.Ticker`. Per the architecture spec.

**Idempotency (critical):** `createInvoiceFromTemplate` rebuilds business+client
snapshots, parses the template's `line_items` JSON, computes subtotal/tax/total,
inserts invoice+lines, AND advances `next_due` — all in ONE transaction.
`next_due` must be advanced in the same txn as the invoice insert, and the sweep
must select only `next_due <= today` (matching the old `getDueTemplates`), so a
crash mid-sweep or a re-run never double-generates. `next_due` past today is the
dedup key.

## Snapshots & numbering (Batches 3–4)

- Numbering: invoice/estimate numbers are generated server-side with transactional
  uniqueness (unique constraint already on `invoice_number`). Port the old
  format/sequence logic.
- Snapshots: on invoice/estimate create, capture `business_snapshot` (from
  business_profile), `client_snapshot`, `payer_snapshot` as JSON, so historical
  documents are immutable against later edits — matches the old behavior.

## Import/Export (Batch 8)

- **Export** (`src/routes/api/export/{invoices,estimates,catalog}`): invoices,
  estimates, AND catalog. CSV via `encoding/csv`; Excel via excelize where the old
  app produces xlsx.
- **Import** (catalog only): CSV **and** Excel (excelize), driven by
  `column_mappings` which carry `tier_mapping` + `metadata_mapping`. Port the
  old **diff → commit** flow (`src/lib/import/{diff-catalog,commit-catalog}.ts`,
  `src/lib/csv/*`, `src/routes/api/import/catalog`): parse → map columns → diff
  against existing catalog → present changes → commit selected.

## Out of scope

- Dashboard, Reports (deferred; easy to add later onto the same pattern).
- AI chat / skills / sub-agents (dropped).
- Removal of the old `src/` Electron tree — happens in a FINAL cleanup batch after
  parity is reached, not per-batch.

## Testing & acceptance (every batch)

- Go: `go test ./... -race`, `go vet ./...`, `gofmt -l` (non-web) clean.
- Frontend: `npm run build` (emits `web/build/200.html`), `npm run check` 0
  errors/0 warnings.
- Live smoke: boot the built binary, curl the new endpoints (auth'd), confirm
  CRUD + SSE event for the new domain.
- NASA Power-of-10 (Go) rules continue to apply.

## Execution

Sequential, autonomous: write Batch N plan → subagent-driven implementation
(fresh subagent per task, spec + code-quality review each) → batch acceptance →
Batch N+1. Surface to the user at batch boundaries.
