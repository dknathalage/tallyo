# Deep Refactor: Vertical-Slice Architecture + Billing Core

- **Status:** Approved (design); ready to plan/implement
- **Date:** 2026-06-18
- **Branch:** `feat/shifts-lifecycle`
- **Scope:** Backend Go reorganization only. **No HTTP API, route, or DB schema changes.** Frontend untouched.

## 1. Motivation

The backend (~15k LOC hand-written Go, excl. `db/gen` + tests) is a healthy layered
monolith — `http → service → repository → db/gen`, with clean downward layering
(repository never imports service/http) and low cross-domain coupling (no service
imports another service). But two structural problems limit it as a foundation:

1. **Domain types have no home.** They live in `internal/repository`
   (`repository.Invoice`, `repository.ShiftInput`, …) and leak upward: 11 HTTP
   handlers import `repository` directly just for the structs. `repository` is
   secretly the domain-model package *and* the data layer.
2. **invoice ≈ estimate duplication.** `repository/invoice.go` (756 LOC) and
   `estimate.go` (746 LOC) are near-twins (line items, snapshots, numbering,
   status, CRUD), and `EstimatesRepo` hacks reuse by embedding `InvoicesRepo`
   (`snap *InvoicesRepo`). ~1500 LOC of parallel code that wants one shared core.

Plus a **navigability tax**: every domain is smeared across `http/ + service/ +
repository/`. To understand "invoice" you open 3 packages.

**Goal:** a solid, scalable foundation via vertical domain slices + a shared
billing core, behavior-preserving and test-gated.

## 2. Decisions (locked)

- **Vertical domain slices** (Option A): one package per domain holding its
  handler + service + repo + types.
- **Unify invoice & estimate on a shared `billing` core.** Share the *Go code*,
  **not** the DB tables (no schema merge — divergent nullability/columns make a
  merged table messy and high-risk).
- **`internal/db/gen` stays ONE central package.** sqlc cannot cleanly split it:
  cross-domain enrichment joins (invoice↔participant, participant↔plan_manager,
  stats↔payments), the global non-tenant catalogue tables, and the single
  177-method `Querier` would force duplicated models or domain→domain imports.
  The generated data-access layer is **platform infrastructure**.
- **Do NOT relocate** `audit / numbering / realtime / reqctx / db` under a
  `platform/` folder — already clean cross-cutting packages; moving them churns
  ~300 import sites for no structural gain. Only new platform package is `httpx`.

## 3. The key enabler: central `gen` dissolves most "hard" deps

The domain audit flagged shift→invoice, payment→invoice, recurring→invoice,
participant→plan_manager as blockers. Almost all are **cross-table reads** (joins,
name enrichment, snapshots). Because `gen` is central, any slice reads any table
through `gen` with **zero domain→domain imports**:

- participant→plan_manager (name) → `gen.ListParticipants` (LEFT JOIN). No dep.
- snapshot building (recurring/invoice read business/participant/plan_manager) →
  billing core queries `gen` directly. No dep.

What remains is only genuine **cross-domain writes / behavior**:

| Coupling | Nature | Resolution |
|---|---|---|
| invoice ↔ shift | **bidirectional**: invoice status/delete cascades to shifts; shift `MarkDrafted` verifies invoice exists | invoice declares `ShiftLinker` iface; shift declares `InvoiceChecker` iface; `app` wires both — package cycle broken |
| recurring → invoice creation | recurring generates invoice rows + line items in one tx | recurring depends on **billing core** (shared), not the invoice slice |
| payment → invoice | FK + paired events + routes nested under `/invoices/{id}/payments` | payment is part of the invoice aggregate → **folded into the invoice slice** |

## 4. Target structure

```
internal/
  db/           gen (CENTRAL) + queries + migrations + sqlite + migrate   ── platform
  audit/ numbering/ realtime/ reqctx/                                       ── platform (paths unchanged)
  httpx/        WriteJSON/WriteError/DecodeJSON/Require*/ParseID/SPAHandler ← extracted from internal/http
  billing/      BillingDocumentFields, LineItem(+Input), computeTotals/round2,
                snapshotJSON + snapshot builders (read gen), numbering glue,
                LineValidator (moved from service/validation.go)
  invoice/      repo+service+handler+types; composes billing; +payment; declares ShiftLinker
  estimate/     repo+service+handler+types; composes billing
  recurring/    repo+service+handler; composes billing for generation
  shift/        repo+service+handler+types; declares InvoiceChecker
  participant/ planmanager/ taxrate/ businessprofile/ customitem/ catalog/
  auth/         already clean → slice as-is
  agent/        cohesive; tools take INTERFACES (InvoiceWriter/ShiftReader/CatalogueReader)
  app/          composition root (wiring + the 3 sweeps), from main.go
main.go          ~40 LOC: flags, config, app.Run
```

### Dependency rule

- `slice → platform` (db/gen, audit, numbering, realtime, reqctx, httpx): allowed.
- billing-related slices (invoice, estimate, recurring) `→ billing`: allowed.
- `slice → slice`: **forbidden.** Cross-domain reads go via central `gen`;
  cross-domain writes/behavior go via interfaces declared by the *consumer* and
  wired in `app`.
- `agent → ` domain interfaces it declares; concrete impls wired in `app`. No
  business domain imports `agent` (zero reverse deps — preserved).

### Route self-registration

Each slice exposes `func (h *Handler) Routes(r chi.Router)`; `app` mounts them
under the authenticated group. Adding a domain no longer touches a central
`server.go` (today: a 25-field `Deps` struct + 150-line route fn with per-handler
nil-checks). Cross-domain route placements stay where they are
(`/invoices/{id}/payments`, agent's `/participants/{id}/draft-invoice`, invoice's
`/participants/{id}/stats`) — chi lets any slice mount any path.

## 5. Billing core seam (invoice/estimate)

Invoice & estimate share ~80%: CRUD, line items, snapshots, numbering, validation,
audit, events. They diverge in exactly three places:

1. **Date field:** invoice `due_date` vs estimate `valid_until`.
2. **Status lifecycle:** `draft→sent→paid/overdue` vs `draft→accepted→converted`.
3. **Derived/cascade behavior:** invoice has payments + overdue sweep + shift
   cascade + `ParticipantStats`; estimate has `Duplicate` + `Convert`.

**Seam = shared embedded struct + shared helpers (no table merge).**

```go
// internal/billing
type DocumentFields struct {
    ID, ... int64
    UUID, Number string
    PlanManagerID *int64
    Status, IssueDate string
    Subtotal, Tax, Total float64
    Notes string
    BusinessSnapshot, ClientSnapshot, PayerSnapshot string
    CreatedAt, UpdatedAt string
    LineItems []*LineItem
}
type LineItem struct { /* fully shared, today in repository/invoice.go */ }
type LineItemInput struct { /* fully shared */ }

func ComputeTotals(items []LineItemInput, taxRate float64) (subtotal, tax, total float64)
func SnapshotJSON(...) string
// snapshot builders read gen directly (business/participant/plan_manager)
type SnapshotBuilder struct { db *sql.DB }
type LineValidator struct { /* moved from service/validation.go */ }
```

Invoice and estimate each keep their **typed sqlc repo** (gen stays per-table) but
embed `DocumentFields` in their domain struct and call the billing helpers —
eliminating the duplication and the `EstimatesRepo`-embeds-`InvoicesRepo` hack.
Recurring uses the billing create+snapshot helpers for generation.

## 6. Phased migration

Every phase must be green before the next:
`go test ./... && go vet ./... && gofmt -l . && (cd web && npm run check && npm run build)`.
Behavior-preserving throughout; **frontend untouched** (API contract stable) —
the primary de-risker.

- **Phase 0 — Extract `httpx`.** Pull domain-agnostic HTTP helpers out of package
  `httpapi` into `internal/httpx` (export `ParseID`, `WriteJSON`, `WriteError`,
  `DecodeJSON`, `WriteValidationError`, `Recover`, `RequestLogger`, `RequireAuth`,
  `RequireRole`, `RequirePlatformAdmin`, logging helpers, `SPAHandler`). Handlers
  stay in `httpapi`, now importing `httpx`. Mechanical, no behavior change.
- **Phase 1 — Billing core.** Create `internal/billing`; move shared
  types/helpers/snapshot builders/validator; refactor invoice & estimate onto it;
  delete the embed hack. Highest-value dedup, before any package reshuffle.
- **Phase 2 — Leaf slices.** Move self-contained domains one at a time into
  `internal/<domain>/`: taxrate, businessprofile, customitem, catalog,
  planmanager, participant. Each self-registers routes. Test after each.
- **Phase 3 — Coupled slices via interfaces.** invoice(+payment), estimate, shift,
  recurring. Wire `ShiftLinker` / `InvoiceChecker` in `app`.
- **Phase 4 — agent + auth + `app`.** auth→slice; agent tools rebind to interfaces
  (`InvoiceWriter`, `ShiftReader`, `CatalogueReader`) instead of concrete service
  types; extract wiring + the 3 sweeps (overdue, recurring, agent) into
  `internal/app`; shrink `main.go` to ~40 LOC.
- **Phase 5 — Cleanup.** Delete emptied `service`/`repository`/`httpapi` packages;
  rewrite the CLAUDE.md architecture section; archive the 2 obsolete notes specs
  (`2026-06-16-notes-journal-*`, `2026-06-17-notes-invoice-harness-*`).

### Independent side task (off the critical path)

DB relocation to cwd + rename (`tallyo-go.db`→`tallyo.db`) and collapsing the dead
notes migrations (`00003_notes.sql` create + `00005_drop_notes.sql` drop net to
nothing; safe to collapse only against a fresh goose history, i.e. the DB rename).
Slot into Phase 5 or do standalone.

## 7. Risks & mitigations

- **Scale:** reorganizes ~15k LOC, requalifies ~500 same-package helper call-sites
  (`WriteJSON` → `httpx.WriteJSON`). Mostly mechanical. *Mitigation:* phased,
  each landing green and independently reviewable; multi-session.
- **invoice↔shift cycle:** resolved by consumer-declared interfaces, not a shared
  package. Verify no residual concrete cross-import after Phase 3.
- **Hidden cross-table writes:** audit confirmed reads dominate; the only
  cross-domain *writes* are the three in §3. If a fourth surfaces, apply the same
  interface pattern.
- **Regression:** backend has tests; the full gate runs each phase. No API/route/
  schema change means the SPA and existing integration behavior are the oracle.

## 8. Out of scope

- Frontend refactors (separate effort; the earlier frontend redundancy audit —
  `formatMoney` dedup, status-badge dedup, component splits — is tracked
  separately).
- Service-layer flattening of pass-through reads (conflicts with the layered
  convention; not pursued).
- The async durable job-queue feature (deferred; this refactor precedes it).
