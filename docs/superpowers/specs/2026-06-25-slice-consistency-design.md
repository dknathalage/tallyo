# Slice Consistency & Boundary Cleanup — Design

**Date:** 2026-06-25
**Status:** Approved (brainstorm), pending spec review + plan
**Goal:** Make the backend easy to manage by giving every domain slice **one identical shape** ("learn one slice, you know them all"), fixing the cross-slice boundary leaks, and optimizing the layout for both human and agent (Claude) navigation.

## Why

A structural audit (3 exploration passes) found:

- **~90% of handler/service/repository code is structurally identical** across the ~7 simple slices (`client`, `payer`, `taxrate`, `customitem`, …) — but with small, gratuitous divergences that make each slice feel slightly different.
- **Validation is smeared across layers.** Handler checks `name==""`, the repo checks `name==""` *again*, and `invoice`/`estimate` use the billing validator — three different homes.
- **Error signalling is inconsistent.** Some services return `(nil, nil)` for not-found, some use sentinels, most failures collapse to a bare 500. Three slices hand-copy a local `writeValidationError`.
- **A documented helper is a lie.** CLAUDE.md lists `httpx.WriteValidationError`; it does not exist.
- **60 inline `realtime.Event{...}` literals** repeat the same four-field construction.
- **The "no slice imports another slice" rule is violated in production.** `estimate` and `recurring` import the `invoice` package to reuse `NextInvoiceNumber` and `InsertLineItems`, which are really shared billing-document mechanics living in the wrong package. (All *other* cross-slice imports are test-only; production is otherwise clean.)

The fix is **consistency and correct boundaries**, NOT a generic CRUD engine. A `CRUDService[T]`/`CRUDRepository[T]` base was explicitly rejected: in Go the per-domain work *is* the field mapping + concrete sqlc calls, so generics would replace readable boilerplate with a struct of function pointers — more code, worse stack traces, the opposite of "easy to manage."

## Decisions (locked during brainstorm)

1. **Win = consistency**, not DRY-via-machinery. Standardize the shape; keep slices explicit.
2. **Validation lives in the service** — once, before the repo. Handler decodes + maps HTTP; repo trusts its input.
3. **Sentinel errors + one shared mapper.** Services return typed/sentinel errors; one `httpx.WriteServiceError` maps them to HTTP. Every handler's error handling becomes one identical line.
4. **Fix the boundaries** — relocate the shared billing mechanics into `internal/billing`; `estimate`/`recurring` stop importing `invoice`.
5. **Flat layout, no grouping folder.** `internal/<slice>/` stays flat. Effort goes into *naming discipline + file splitting*, not tree depth — this is what makes the codebase fastest for Claude to navigate (grep/glob/symbol search, predictable paths, files that fit one Read).

## The canonical slice

Every slice is **one flat Go package**, organized by file (never by layer-subpackage — that forces export-everything and invites import cycles). The contract:

```
internal/<slice>/
  handler.go      HTTP only — decode, call service, map result. No validation.
  service.go      The brain — validate input, orchestrate, broadcast. Returns typed errors.
  repository.go   Thin — audit.WithTx + gen call + row→domain map. Trusts its input.
  query.go        (optional) list/filter read SQL, split out when repository.go grows.
  types.go        domain struct + Input struct + Input.Validate() error.
```

### handler.go — uniform, ~4 lines per method

```go
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    var in XInput
    if err := httpx.DecodeJSON(r, &in); err != nil { httpx.WriteError(w, 400, "invalid request"); return }
    out, err := h.svc.Create(r.Context(), in)
    if httpx.WriteServiceError(w, err) { return }
    httpx.WriteJSON(w, 201, out)
}
```

`WriteServiceError` returns `false` on nil error (writes nothing — the happy path falls through), `true` after writing any error response. Get/Update/Delete/List are identical modulo verb + status.

### service.go — owns validation + orchestration

```go
func (s *Service) Create(ctx context.Context, in XInput) (*X, error) {
    if err := in.Validate(); err != nil { return nil, err }      // *billing.ValidationError
    tid := reqctx.MustTenant(ctx)
    x, err := s.repo.Create(ctx, tid, in)
    if err != nil { return nil, err }
    s.events.Created(ctx, x.ID)                                  // broadcast helper
    return x, nil
}
```

Not-found propagates as `apperr.ErrNotFound` (repo raises it, service passes it up untouched). Conflicts use a slice-local sentinel (`var ErrAlreadyConverted = …`). Real domain logic (invoice line-items, recurring generation, status cascades) sits visibly here — that divergence is expected and welcome.

### repository.go — thin, trusts input

```go
func (r *Repo) Update(ctx context.Context, tid, id string, in XInput) (*X, error) {
    var out gen.X
    err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
        x, e := gen.New(tx).UpdateX(ctx, gen.UpdateXParams{ /* map in */ ID: id, TenantID: tid })
        if errors.Is(e, sql.ErrNoRows) { return apperr.ErrNotFound }
        if e != nil { return fmt.Errorf("update: %w", e) }
        out = x
        return audit.Log(ctx, tx, audit.Entry{EntityType: "x", EntityID: x.ID, Action: "update",
            Changes: audit.Changes(map[string]any{"name": in.Name})})
    })
    if err != nil { return nil, err }
    return toX(out), nil
}
```

No `name==""` checks (moved to the service). The repo's only jobs: persist, audit, translate `sql.ErrNoRows` → `apperr.ErrNotFound`.

## Platform additions (shared homes)

| New | Where | Replaces |
|---|---|---|
| `ErrNotFound`, `ErrConflict` sentinels | `internal/apperr/` | ambiguous `(nil, nil)` |
| `WriteServiceError(w, err) bool` | `internal/httpx/` | per-handler `errors.Is` chains + bare 500s |
| `WriteValidationError(w, err)` (make real) | `internal/httpx/` | 3 copied local `writeValidationError` |
| `events.Notifier{hub, entity}` with `Created/Updated/Deleted(ctx, id)` | `internal/events/` | 60 inline `realtime.Event{}` literals |
| `Input.Validate() error` per slice | each `types.go` | repo + handler double-checks |

**`apperr` as its own package** (not inside `httpx`): the mapper must recognize `billing.ValidationError`. If `httpx` imported `billing` the whole HTTP layer takes a fat dependency. A tiny `apperr` package holds the sentinels and a structural `Validation` interface that `billing.ValidationError` satisfies, keeping `httpx` thin.

`events.Notifier` is a 3-method struct (no generics, no base class) — each service constructs `events.New(hub, "client")` once and calls `s.events.Updated(ctx, id)`. It reads the tenant from `ctx`. **Caveat from spec review:** background sweeps (`app/sweep.go`) broadcast tenant-scoped events outside an HTTP request, where `reqctx.MustTenant(ctx)` would panic. Sweep-path broadcasts must either run under a ctx that carries the tenant, or use an explicit-tenant `events.NotifierFor(tenantID)` variant. Verify each broadcast call site's ctx before swapping it to the notifier; keep explicit-tenant broadcasts explicit.

### Not-found layering (refined in spec review)

The "no ambiguous `(nil, nil)`" rule applies to the **HTTP CRUD boundary**, not to every internal read:

- `repo.Get(...)` stays a plain lookup returning `(nil, nil)` on a missing row — Go-idiomatic and reusable by internal skip-on-missing callers (e.g. `recurring.GenerateOne` skips a not-due/absent template, `repository.go:440`).
- The **CRUD service** `Get` translates a `(nil, nil)` repo result into `apperr.ErrNotFound`; the handler maps it to 404 via `WriteServiceError`.
- `repo.Update` / `repo.Delete` raise `apperr.ErrNotFound` directly from `sql.ErrNoRows` (a mutation must hit an existing row; no internal caller wants skip-on-missing here).

This keeps handlers uniform without breaking the recurring generation path.

## Boundary relocation

`estimate.convertTx` and `recurring.generateTx` each build an invoice **inline, in their own transaction**, co-transacted with `SetEstimateConverted` / `SetRecurringNextDue` for atomicity. So an `InvoiceCreator` service interface is the **wrong** abstraction — it can't share the caller's `*sql.Tx`, and splitting the work into two transactions would break the atomic convert/generate. (This was caught in spec review; the earlier `InvoiceCreator` idea is rejected, see Non-goals.)

The actual cross-slice coupling is narrow: both reach into the `invoice` *package* only for two shared mechanics. Move those into `internal/billing`:

```
internal/invoice/repository.go            internal/billing/
  NextInvoiceNumber(...)         ───────►   numbering.go   NextNumber(ctx, q, tid, prefix)
  InsertLineItems(...)           ───────►   lineitems.go   InsertLineItems(ctx, q, tid, invoiceID, items)
```

- `NextNumber` gains a `prefix` arg (`"INV-"`, `"EST-"`) — genuinely shared, not invoice-flavored. `invoice` keeps a one-line `NextInvoiceNumber("INV-")` wrapper.
- `InsertLineItems` serves the **`line_items` table only** (used by `invoice` and `recurring`). It is **not** folded with estimate's line copy: `estimate.convertTx` already uses its own `copyEstimateItemsToInvoice`, and `estimate_line_items` has an incompatible column set (`CreateEstimateLineItemParams` is 15 fields vs `CreateLineItemParams` 18). Forcing one function behind a `table` param was rejected — different shapes, keep them separate.

After the move:
- `estimate` imports only `billing` (+ shared `gen`) — its sole `invoice` import (`NextInvoiceNumber`, line 674) is gone.
- `recurring`'s three `invoice` uses are removed: `NextInvoiceNumber` → `billing.NextNumber`, `InsertLineItems` → `billing.InsertLineItems`, and the read-back `invoice.NewInvoices(r.db).Get(ctx, …, invID)` (line 457) → map the `inv` (`gen.Invoice`) row `generateTx` **already produced in-tx** into the return type (no separate read, no `invoice` import).

The "no slice imports another slice" rule then holds in production — with **no new interface**, no transaction split.

## Layout — flat, agent-optimized

```
internal/
  client/   handler.go service.go repository.go query.go types.go
  payer/    handler.go service.go repository.go types.go
  invoice/  handler.go service.go repository.go query.go payment_repository.go types.go
  …
  billing/  lineitem.go totals.go validation.go snapshot.go numbering.go lineitems.go
  httpx/ events/ apperr/ audit/ realtime/ numbering/ ids/ reqctx/ db/   ← platform, flat
  app/      app.go server.go sweep.go wire.go auth_handlers.go
```

**No `internal/domain/…` or `internal/platform/…` grouping wrapper.** Rationale (agent navigation): Claude navigates by grep/glob/symbol search and *path guessing*, not tree browsing. The wins, ranked:

1. **Rigid filename consistency** — every slice is exactly `handler.go / service.go / repository.go / types.go`. Opening the right file is a first-try guess; no `ls` needed.
2. **Shallow flat paths** — `internal/client/service.go` beats `internal/domain/client/service.go`. Every extra level is a wrong-guess opportunity. This is why the grouping folder is rejected.
3. **Files fit one Read (≤ ~400 lines)** — split big files on predictable seams (`query.go`, `payment_repository.go`) so Claude loads only the needed logic, not a 994-line repo.
4. **A slice-anatomy contract in CLAUDE.md** — one paragraph telling the shape of any slice before it's opened.
5. **Greppable, unique symbol names** — one search hit, not twelve aliased `Service`s.

## Migration order

Each step compiles and is green (`go test ./... -race && go vet ./... && gofmt -l .`) before the next; each is an independently reviewable commit.

1. **Platform additions** — add `apperr`, `httpx.WriteServiceError`, `httpx.WriteValidationError`, `events.Notifier`. Pure additions, nothing consumes them yet. Zero risk.
2. **Relocate billing mechanics** — move `NextNumber(…, prefix)` + `InsertLineItems` (line_items table) into `billing`; point invoice's `NextInvoiceNumber` wrapper + estimate + recurring at them. Estimate's own `copyEstimateItemsToInvoice` is left as-is (not folded).
3. **Drop `invoice` imports from estimate/recurring** — estimate now uses `billing.NextNumber("EST-"/"INV-")`; recurring uses `billing.NextNumber`/`billing.InsertLineItems` and returns the in-tx `gen.Invoice` row instead of reading it back via `invoice.NewInvoices(...).Get`. No new interface. Cuts the last production cross-slice edge. `go list -deps`/`go vet` confirms estimate and recurring no longer import `internal/invoice`.
4. **Conform slices to the canonical shape**, one slice per commit, simplest first:
   `taxrate → payer → customitem → client → businessprofile → session → recurring → estimate → invoice`.
   Each: validation → service, sentinel errors + mapper, `events` helper, thin the repo, split files > ~400 lines.
5. **Update CLAUDE.md** — document the now-true canonical shape + filename contract so the next slice added copies it. Correct the stale `WriteValidationError` claim.

## Non-goals / rejected

- Generic `CRUDService[T]` / `CRUDRepository[T]` base classes — adds indirection, the opposite of the goal.
- An `InvoiceCreator` interface for convert/generate — rejected in spec review: both are co-transactional inline invoice builds, and an interface call can't share the caller's `*sql.Tx` without breaking atomicity. Relocating the two shared mechanics to `billing` removes the coupling instead.
- Folding estimate's line-item insert into a `table`-parameterized `InsertLineItems` — rejected: `estimate_line_items` (15 cols) and `line_items` (18 cols) have incompatible shapes; estimate keeps `copyEstimateItemsToInvoice`.
- A centralized error-mapper *registry* — only 2-3 handlers have `errors.Is` chains; `WriteServiceError`'s fixed switch is enough.
- Removing `reqctx.MustTenant` repetition — it's the multi-tenant safety reflex, intentionally explicit; hiding it is a security regression.
- `internal/domain/` or `internal/platform/` grouping folders — optimize for human tree-browsing, cost Claude navigation.
- Touching the frontend — no `web/` changes expected.

## Testing

- Per-slice service tests can now exercise validation **without HTTP** (validation moved into the service) — a direct win of decision 2.
- The full gate stays `go test ./... -race`, `go vet ./...`, `gofmt -l .`.
- Behavior is preserved per slice; conformance commits are refactors, not feature changes — existing tests must stay green at each step.
