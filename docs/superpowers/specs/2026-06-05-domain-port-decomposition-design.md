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
   `src/lib/db/number-generators.ts` + `src/lib/utils/{invoice,estimate}-number.ts`
   (transactional uniqueness). Needed by Batches 3–4.
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
| 1 | clients, payers | 0 |
| 2 | tax-rates, rate-tiers, catalog (+catalog_item_rates) | 0 |
| 3 | invoices (+line_items, numbering, status, snapshots) | 1, 2 |
| 4 | estimates (+estimate_line_items, numbering) | 1, 2 |
| 5 | payments (linked to invoices, paid/AR rollup) | 3 |
| 6 | recurring-templates (+scheduling: run-on-launch sweep + ticker) | 3, 4 |
| 7 | PDF generation (maroto) for invoices + estimates | 3, 4 |
| 8 | import/export (CSV/Excel export; CSV catalog import + column_mappings) | 2, 3, 4 |

Dependencies confirmed from the schema: `invoices→clients, tax_rates`;
`line_items→invoices(cascade), catalog_items, rate_tiers`;
`estimates→clients, tax_rates`; `estimate_line_items→estimates(cascade)`;
`payments→invoices(cascade)`; `catalog_item_rates→catalog_items, rate_tiers`.
Invoices/estimates also store `business_snapshot`/`client_snapshot`/
`payer_snapshot` JSON (denormalized point-in-time copies) — port that behavior.

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

## Snapshots & numbering (Batches 3–4)

- Numbering: invoice/estimate numbers are generated server-side with transactional
  uniqueness (unique constraint already on `invoice_number`). Port the old
  format/sequence logic.
- Snapshots: on invoice/estimate create, capture `business_snapshot` (from
  business_profile), `client_snapshot`, `payer_snapshot` as JSON, so historical
  documents are immutable against later edits — matches the old behavior.

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
