# Vertical-Slice Refactor — Phase 0 & 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the domain-agnostic HTTP helpers into `internal/httpx` (Phase 0), then build the shared `internal/billing` core and refactor invoice + estimate onto it, removing the duplication (Phase 1).

**Architecture:** Behavior-preserving backend reorganization. No HTTP API, route, or DB schema changes; frontend untouched. `internal/db/gen` stays the single central data-access package. Phase 0 uses **compatibility aliases** in package `httpapi` so existing handler call-sites don't churn — they migrate when each domain becomes a slice in later phases.

**Tech Stack:** Go 1.26, chi v5, modernc SQLite + sqlc, goose. Test gate per task: `go build ./... && go test ./... && go vet ./... && gofmt -l .` (expect empty `gofmt` output). Frontend gate (`cd web && npm run check && npm run build`) runs once per phase since no web files change.

**Spec:** @docs/superpowers/specs/2026-06-18-vertical-slice-architecture-design.md

**TDD note:** Most tasks move already-tested code; the discipline is *the existing suite stays green after every commit*. New surface (the `billing` package API) gets unit tests written test-first. **Every commit in this plan builds the whole module** — there are no intentionally-broken checkpoints.

---

## Scope decisions locked by the plan review (read before starting)

- **`LineValidator` does NOT move in Phase 1.** It holds `*repository.CatalogRepo`, `*repository.BusinessProfileRepo`, `*repository.ParticipantsRepo`, `*repository.TaxRatesRepo` (`internal/service/validation.go:98-102`). Moving it to `billing` while `repository` imports `billing` (for the line-item types) would create a `billing → repository → billing` import cycle. It stays in `internal/service` for now; its method/result types simply switch to `billing.LineItemInput`. Moving it is a later-phase task that first rewrites those four repo deps to read `gen` directly.
- **`WriteValidationError` does NOT move to `httpx` in Phase 0.** It depends on `service.AsValidationError` (`internal/http/respond.go:47`). It stays in package `httpapi` (calling `httpx.WriteJSON` internally). It moves to `httpx` only after validation lands in `billing` (later phase).
- **`computeTotals(items, tax)` takes an already-computed absolute tax amount** (`internal/repository/invoice.go:147-155`), not a rate. Tests and call-sites must preserve that semantics.

---

## File Structure

**Phase 0 creates:**
- `internal/httpx/respond.go` — `WriteJSON`, `WriteError`, `DecodeJSON` (NOT `WriteValidationError`)
- `internal/httpx/parseid.go` — `ParseID` (exported; was unexported `parseID`)
- `internal/httpx/middleware.go` — `Recover`, `RequestLogger`, `RequireAuth`, `RequireRole`, `RequirePlatformAdmin`, `UserFrom`, `statusWriter`
- `internal/httpx/logging.go` — `WithLogger`, `EnrichLogger`, `LoggerFrom`
- `internal/httpx/static.go` — `SPAHandler`, `readFile`, `serveBytes`
- `internal/http/aliases.go` — shims so `httpapi` handlers keep compiling unchanged

**Phase 0 keeps in `internal/http`:** `respond.go` retains `WriteValidationError` (depends on `service`).

**Phase 1 creates:**
- `internal/billing/lineitem.go` — `LineItem`, `LineItemInput` (moved from `repository/invoice.go:62,97`)
- `internal/billing/totals.go` — `Totals`, `ComputeTotals`, `Round2`
- `internal/billing/snapshot.go` — `SnapshotJSON` + `SnapshotBuilder` (reads `gen` only)
- `internal/billing/*_test.go`

**Phase 1 modifies (line-item type ripple — confirm via grep, do not trust a static list):**
`repository/invoice.go`, `repository/estimate.go`, `repository/recurring.go`, `service/*.go` (incl. `validation.go`), `agent/tools_invoice.go`, `internal/pdf/pdf.go`, `internal/http/invoices.go`, `internal/http/estimates.go`, and **all `_test.go`** that reference the types. Note: `Invoice.LineItems` / `Estimate.LineItems` are `[]*LineItem` (`invoice.go:58`, `estimate.go:54`) — their exported field type becomes `[]*billing.LineItem`, rippling to every reader of `inv.LineItems` (e.g. `pdf/pdf.go:112`).

---

# Phase 0 — Extract `internal/httpx`

### Task 0.1: Move the leaf helpers (respond/parseid/logging/static)

**Files:** Create `internal/httpx/{respond,parseid,logging,static}.go`; modify the `internal/http` originals.

- [ ] **Step 1: Move `WriteJSON`, `WriteError`, `DecodeJSON` into `internal/httpx/respond.go`** (`package httpx`), bodies verbatim. Also move the `maxRequestBody` const (`internal/http/respond.go:17`) that `DecodeJSON` depends on. **Leave `WriteValidationError` in `internal/http/respond.go`** (it imports `service`); have it call `httpx.WriteJSON` (add the `httpx` import to the `http` file). After this, `internal/http/respond.go` contains only `WriteValidationError`.

- [ ] **Step 2: Move `logging.go` and `static.go` verbatim** into `internal/httpx` (`package httpx`). Move `parseID`→`ParseID` (capital P) into `internal/httpx/parseid.go`.

- [ ] **Step 3: Delete the moved definitions from the `internal/http` originals.**

- [ ] **Step 4: Build** — `go build ./internal/httpx/`
Expected: compiles (stdlib + `internal/reqctx` only).

- [ ] **Step 5: Commit**
```bash
git add internal/httpx/ internal/http/
git commit -m "refactor(httpx): move WriteJSON/WriteError/DecodeJSON/ParseID/logging/static"
```
(Module won't fully build until Task 0.3 adds aliases — but do NOT push between 0.1–0.3; treat 0.1–0.3 as one logical unit. If you prefer a building commit, do Steps in 0.1–0.3 then a single commit at 0.3.)

### Task 0.2: Move the middleware

**Files:** Create `internal/httpx/middleware.go`; modify `internal/http/middleware.go`.

- [ ] **Step 1: Move `Recover`, `RequestLogger`, `statusWriter`, `RequireAuth`, `RequireRole`, `RequirePlatformAdmin`, `UserFrom`** into `internal/httpx/middleware.go` (`package httpx`). Imports `internal/auth`, `internal/reqctx`, `alexedwards/scs/v2`. Verified cycle-free: `internal/auth` imports neither `http` nor `httpx`.

- [ ] **Step 2: Remove the moved code from `internal/http/middleware.go`.**

- [ ] **Step 3: Build** — `go build ./internal/httpx/`
Expected: compiles.

### Task 0.3: Add compatibility aliases + verify whole module

**Files:** Create `internal/http/aliases.go`.

- [ ] **Step 1: Write `internal/http/aliases.go`:**
```go
package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
)

// Compatibility shims: implementations live in internal/httpx. Per-domain
// handlers drop these and call httpx.* directly when they move to slices.
// NOTE: WriteValidationError stays defined in respond.go (it depends on service).
var (
	WriteJSON            = httpx.WriteJSON
	WriteError           = httpx.WriteError
	DecodeJSON           = httpx.DecodeJSON
	Recover              = httpx.Recover
	RequestLogger        = httpx.RequestLogger
	RequireAuth          = httpx.RequireAuth
	RequireRole          = httpx.RequireRole
	RequirePlatformAdmin = httpx.RequirePlatformAdmin
	UserFrom             = httpx.UserFrom
	WithLogger           = httpx.WithLogger
	EnrichLogger         = httpx.EnrichLogger
	LoggerFrom           = httpx.LoggerFrom
	SPAHandler           = httpx.SPAHandler
)

func parseID(r *http.Request) (int64, bool) { return httpx.ParseID(r) }
```
All aliased symbols are non-generic plain funcs / `func(http.Handler) http.Handler` / `http.Handler`-returning values; `var =` aliasing is valid and matches their use as values (`r.Use(Recover)`, `pr.With(RequirePlatformAdmin)`).

- [ ] **Step 2: Full gate** — `go build ./... && go test ./... && go vet ./... && gofmt -l .`
Expected: build clean, tests pass, `gofmt` empty.

- [ ] **Step 3: Frontend sanity** — `cd web && npm run build && cd ..`

- [ ] **Step 4: Commit**
```bash
git add internal/http/ internal/httpx/
git commit -m "refactor(http): extract httpx package; alias helpers to keep handlers unchanged"
```

---

# Phase 1 — Billing core

### Task 1.1: Add `billing` line-item types AND repoint every reference (one building commit)

**Files:** Create `internal/billing/lineitem.go`, `internal/billing/lineitem_test.go`; modify every file referencing the moved types.

- [ ] **Step 1: Write failing test** `internal/billing/lineitem_test.go`:
```go
package billing

import "testing"

func TestLineItemTypes(t *testing.T) {
	var in LineItemInput
	if in.Quantity != 0 || in.GstFree {
		t.Fatalf("unexpected zero value: %+v", in)
	}
	li := LineItem{Code: "01_011", Quantity: 2, UnitPrice: 10}
	if li.Code != "01_011" {
		t.Fatalf("LineItem field mismatch")
	}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./internal/billing/`
Expected: FAIL (types undefined).

- [ ] **Step 3: Move the struct definitions** `LineItem` (`repository/invoice.go:62`) and `LineItemInput` (`:97`) into `internal/billing/lineitem.go` (`package billing`), verbatim (same fields + json tags). Remove them from `repository/invoice.go`.

- [ ] **Step 4: Repoint ALL references.** Find them:
```bash
grep -rln "repository\.LineItem" internal/
grep -rln "LineItem" internal/repository/ internal/pdf/
```
The first grep misses test files that only read `inv.LineItems` (e.g. `internal/agent/tools_invoice_create_test.go`, `internal/http/*_test.go`) — those keep compiling (they never name the type), but for an honest inventory also run `grep -rln "\.LineItems\|repository\.LineItem" internal/ | grep -v internal/db/`.
Replace `repository.LineItem`→`billing.LineItem` and `repository.LineItemInput`→`billing.LineItemInput` at every external site; inside package `repository` (invoice/estimate/recurring) use `billing.LineItem(Input)` and add the `billing` import; change the `Invoice.LineItems` / `Estimate.LineItems` field types to `[]*billing.LineItem`. Cover `internal/pdf`, `internal/http/{invoices,estimates}.go`, `internal/service/*` (incl. `validation.go`'s `ValidationResult.Items` and method params), `internal/agent/tools_invoice.go`, and every `_test.go` the greps surface.

- [ ] **Step 5: Build + test** — `go build ./... && go test ./...`
Expected: PASS (whole module builds). Resolve any missed reference until green.

- [ ] **Step 6: Format + vet + commit**
```bash
gofmt -w .
go vet ./...
git add -A
git commit -m "feat(billing): add shared LineItem(Input); repoint all references"
```

### Task 1.2: Move totals helpers into `billing`

**Files:** Create `internal/billing/totals.go`, `internal/billing/totals_test.go`; modify `repository/invoice.go` (remove `totals`/`computeTotals`/`round2`) and the four call-sites.

- [ ] **Step 1: Write failing test** matching the real "tax is a pre-computed absolute amount" semantics:
```go
func TestComputeTotals(t *testing.T) {
	items := []LineItemInput{{Quantity: 2, UnitPrice: 10}, {Quantity: 1, UnitPrice: 5}}
	got := ComputeTotals(items, 10) // 10 = absolute tax amount, NOT a rate
	if got.Subtotal != 25 || got.Tax != 10 || got.Total != 35 {
		t.Fatalf("ComputeTotals = %+v, want {25 10 35}", got)
	}
}
func TestRound2(t *testing.T) {
	if Round2(1.005) != 1.01 {
		t.Fatalf("Round2(1.005) = %v, want 1.01", Round2(1.005))
	}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./internal/billing/ -run 'ComputeTotals|Round2'`

- [ ] **Step 3: Move + export.** Move `round2`→`Round2`; move `computeTotals`→`ComputeTotals` returning an exported `type Totals struct { Subtotal, Tax, Total float64 }` (same body, exported fields). **⚠ Do NOT blind-replace `.subtotal`→`.Subtotal`** — the unrelated row-mapping structs `invoiceFields`/`estimateFields` have identically-named lowercase fields accessed as `f.subtotal/.tax/.total` (`invoice.go:644,664-666`; `estimate.go:656-658`) and must stay lowercase. Edit only accesses on the `ComputeTotals` *result* variable (the `t :=` totals value). Update the four call-sites and their result-field accesses:
  - `repository/invoice.go:252,427` (and reads at `:263-265`)
  - `repository/estimate.go:181,350`
  - `repository/recurring.go:284` (`ComputeTotals(items, 0).Subtotal`) and `:369`; note `recurring.go:285` computes the tax amount separately and passes it in — keep that.
  Replace remaining `round2(` usages in `repository/` with `billing.Round2(`.

- [ ] **Step 4: Build + test** — `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**
```bash
gofmt -w . && go vet ./...
git add -A
git commit -m "refactor(billing): move ComputeTotals/Round2 into billing core"
```

### Task 1.3: Move snapshot building into `billing`; drop the estimate→invoice embed

**Files:** Create `internal/billing/snapshot.go`, `internal/billing/snapshot_test.go`; modify `repository/invoice.go`, `repository/estimate.go`, `repository/recurring.go`.

- [ ] **Step 1: Write `internal/billing/snapshot.go`** — `SnapshotBuilder struct { db *sql.DB }`, `NewSnapshotBuilder(db *sql.DB) *SnapshotBuilder`, exported `SnapshotJSON(...)` (from `invoice.go:589`), and methods `Business(ctx, tenantID)`, `Participant(ctx, tenantID, participantID)`, `PlanManager(ctx, tenantID, planManagerID *int64)` — bodies moved verbatim from the `InvoicesRepo` builders (`invoice.go:605,614,624`). Verified: these read via `gen.New(r.db)` only — no domain deps, cycle-free.

- [ ] **Step 2: Write a snapshot test** (mirror `internal/agent/store_test.go` setup: `appdb.Open(filepath.Join(t.TempDir(),"t.db"))` + `appdb.Migrate`). Seed a business profile row; assert `NewSnapshotBuilder(db).Business(ctx, tenantID)` returns the expected JSON. Run → fail → implement wiring → pass.

- [ ] **Step 3: Replace usages.**
  - `repository/invoice.go`: hold a `*billing.SnapshotBuilder` on `InvoicesRepo` (construct in `NewInvoices`); `fillSnapshots` calls it. Remove `snapshotJSON` + the three builder methods.
  - `repository/estimate.go`: **delete the `snap *InvoicesRepo` field** (`estimate.go:82`); use a `*billing.SnapshotBuilder` instead.
  - `repository/recurring.go`: replace `r.snap.*` (`recurring.go:312-314`) with a `*billing.SnapshotBuilder`; remove its `snap *InvoicesRepo` (`recurring.go:81`).

- [ ] **Step 4: Build + test** — `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Verify the hack is gone** — `grep -rn "snap *\*InvoicesRepo" internal/` returns nothing.

- [ ] **Step 6: Commit**
```bash
gofmt -w . && go vet ./...
git add -A
git commit -m "refactor(billing): move snapshot builders into billing; drop estimate/recurring->invoice embed"
```

### Task 1.4: Phase 1 verification sweep

- [ ] **Step 1: Full gate** — `go build ./... && go test ./... -race && go vet ./... && gofmt -l . && (cd web && npm run check && npm run build)`
Expected: all green; `gofmt -l` empty.
- [ ] **Step 2: cgo-free binary** — `CGO_ENABLED=0 go build .`
Expected: builds.
- [ ] **Step 3: Confirm `internal/billing` imports only** `database/sql`, `encoding/json`, `math`, `internal/db/gen`, stdlib — NOT `internal/repository`/`service`/`http`/`reqctx` (`go list -deps ./internal/billing` check). This guards the no-cycle invariant.

---

## Phases 2–5 (separate follow-on plan, detailed after Phase 1 lands)

- **Phase 2** — leaf slices: taxrate, businessprofile, customitem, catalog, planmanager, participant.
- **Phase 3** — coupled slices: invoice(+payment), estimate, shift, recurring; wire `ShiftLinker`/`InvoiceChecker` in `app`.
- **Phase 4** — auth slice; agent tools rebind to interfaces; `internal/app` composition root + sweeps; shrink `main.go`. **Includes:** move `LineValidator` to `billing` by first rewriting its four `repository.*Repo` reads to `gen` (removes the `billing→repository` cycle), then move `WriteValidationError`/`AsValidationError` to `httpx`/`billing` and drop the `httpapi` shims.
- **Phase 5** — delete emptied `service`/`repository`/`httpapi`; remove `aliases.go`; rewrite CLAUDE.md architecture section; archive obsolete notes specs; (optional) DB cwd-relocation + rename + collapse notes migrations.
