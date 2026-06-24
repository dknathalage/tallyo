# Single UUIDv7 Identifier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Tallyo's dual-identifier model (int64 PK + `uuid` column on every table) with a single UUIDv7 string id used end to end, killing all int PKs and the `uuid → int` resolution layer.

**Architecture:** Edit the clean-break migrations in place so each table's `id` becomes a `TEXT PRIMARY KEY` holding a UUIDv7 (the old separate `uuid` column is dropped, every FK/guard column becomes TEXT), regenerate sqlc, then sweep the platform packages, every domain slice, and every test from `int64` ids to `string`. One shared `internal/ids.New()` mints every id.

**Tech Stack:** Go 1.26, chi v5, modernc.org/sqlite, sqlc, goose, `google/uuid` (NewV7), Playwright (e2e).

---

## ⚠️ Read first: this is a monolithic sweep

Unlike a normal TDD plan, **the tree does not compile between Phase 1 and the end of Phase 3.** Editing the migrations + regenerating sqlc flips every `gen` model from `int64` to `string`, which breaks every caller at once. There is no green intermediate state until the whole int→string flip lands.

Consequences for execution:
- **Phase 0 is the only independently-green phase before the end** — do it first, commit, verify build.
- Phases 1–3 are committed as reviewable chunks but **will not build in isolation**. That is expected. Do not try to "make the test pass" after each — the gate runs once, in Phase 5.
- Work on a **dedicated branch/worktree** (see Execution Handoff). Do not interleave with other work.
- If using subagent-driven execution, the per-task "verify" for Phases 1–3 is `go build ./...` *narrowed to the package being edited where it already has its deps converted* — or simply a visual diff review. The real gate is Phase 5.

---

## File Structure

| Path | Responsibility | Action |
|------|----------------|--------|
| `internal/ids/ids.go` | The single id generator (`New() → uuidv7`) | Create |
| `internal/db/migrations/control/00001_control.sql` | tenants, users, invites, audit_log → TEXT uuid PK/FK | Modify |
| `internal/db/migrations/tenant/00001_tenant.sql` | all tenant business tables → TEXT uuid PK/FK | Modify |
| `internal/db/migrations/tenant/00002_audit_log.sql` | tenant audit_log → TEXT PK | Modify |
| `internal/db/migrations/tenant/00003_catalogue.sql` | price_list_versions, items → TEXT uuid PK/FK | Modify |
| `internal/db/migrations/tenant/00004_catalogue_tenant.sql` | `tenant_id` guard col → TEXT | Modify |
| `internal/db/queries/*.sql` | id-typed params → string | Modify |
| `internal/db/gen/*` | regenerated (do not hand-edit) | Regenerate |
| `internal/reqctx/reqctx.go` | TenantID/UserID `int64 → string` | Modify |
| `internal/audit/audit.go` | `Entry.EntityID int64 → string` | Modify |
| `internal/httpx/*.go` | remove `ParseID`; keep `ParseUUID` | Modify |
| `internal/app/auth_handlers.go`, `server.go`, middleware | session id strings, `GetString` | Modify |
| `internal/{client,payer,invoice,estimate,recurring,session,taxrate,businessprofile,customitem,pricelist,auth,smarts}/*.go` | repo/service/handler `int64 → string`, drop resolve-to-int | Modify |
| `internal/billing/*.go` | snapshot/line-item id handling → string | Modify |
| `internal/listquery/*.go` | id-typed params → string (no id columns of its own) | Modify |
| `**/*_test.go` | seeds/asserts `int64 → string` | Modify |
| `CLAUDE.md`, `docs/data-model.md`, `docs/gotchas.md` | rewrite UUID-addressing convention | Modify |

---

## Phase 0 — One id generator (independently green)

### Task 0: `internal/ids` + replace all v4 call sites

**Files:**
- Create: `internal/ids/ids.go`
- Create: `internal/ids/ids_test.go`
- Modify: every file with `uuid.NewString()` (74 sites / 41 files)

- [ ] **Step 1: Write the failing test**

```go
// internal/ids/ids_test.go
package ids

import "testing"

func TestNewIsUUIDv7AndOrdered(t *testing.T) {
	a := New()
	b := New()
	if len(a) != 36 {
		t.Fatalf("want 36-char uuid, got %q", a)
	}
	// v7 puts the version nibble at index 14.
	if a[14] != '7' {
		t.Fatalf("want version 7, got %q in %q", a[14], a)
	}
	// v7 is time-ordered: a minted before b must sort before b.
	if !(a < b) {
		t.Fatalf("v7 ids must be lexically time-ordered: %q !< %q", a, b)
	}
}
```

- [ ] **Step 2: Run it, verify it fails to compile** — `go test ./internal/ids/` → FAIL (`undefined: New`).

- [ ] **Step 3: Implement**

```go
// internal/ids/ids.go

// Package ids mints the application's single identifier type: a time-ordered
// UUIDv7 string. Every row id and every generated id goes through New() — there
// is exactly one id convention in the codebase.
package ids

import "github.com/google/uuid"

// New returns a fresh UUIDv7 as a 36-char string. UUIDv7 is time-ordered, so
// ids sort chronologically (preserving the old ORDER BY id behaviour).
func New() string {
	return uuid.Must(uuid.NewV7()).String()
}
```

- [ ] **Step 4: Run it** — `go test ./internal/ids/` → PASS.

- [ ] **Step 5: Replace every call site.** Mechanical: `uuid.NewString()` → `ids.New()`, add the `internal/ids` import, drop the now-unused `github.com/google/uuid` import where it was only used for `NewString`.

```bash
grep -rln "uuid.NewString" internal/   # the 41 files to edit
```

Edit each. Where a file still uses `uuid` for other things (e.g. parsing), keep the import.

- [ ] **Step 6: Verify the sweep is complete and builds**

Run:
```bash
grep -rn "uuid.NewString" internal/   # expected: no output
go build ./... && go vet ./... && gofmt -l .
go test ./... 2>&1 | tail -5
```
Expected: zero `NewString` hits, clean build/vet/fmt, tests still green (this phase is behaviour-preserving).

- [ ] **Step 7: Commit**

```bash
git add internal/ids/ internal/
git commit -m "refactor: mint all ids via internal/ids.New (uuidv7), drop uuid.NewString"
```

---

## Phase 1 — Schema + sqlc regen (build goes RED here)

### Task 1: Rewrite the migrations to TEXT uuid PKs

**Files:** all of `internal/db/migrations/control/*.sql` and `internal/db/migrations/tenant/*.sql`.

**The transformation, per table.** Example — `clients` (tenant/00001):

```sql
-- BEFORE
CREATE TABLE clients (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    tenant_id       INTEGER NOT NULL,
    ...
    payer_id        INTEGER REFERENCES payers(id) ON DELETE SET NULL,
    ...
);

-- AFTER
CREATE TABLE clients (
    id              TEXT PRIMARY KEY,          -- uuidv7, app-supplied
    tenant_id       TEXT NOT NULL,             -- guard column (uuid)
    ...
    payer_id        TEXT REFERENCES payers(id) ON DELETE SET NULL,
    ...
);
```

Rules applied to **every** table:
- `id INTEGER PRIMARY KEY AUTOINCREMENT` + `uuid TEXT NOT NULL UNIQUE` → a single `id TEXT PRIMARY KEY`. Delete the `uuid` column.
- Every `*_id INTEGER [NOT NULL] REFERENCES x(id) ...` → `*_id TEXT [NOT NULL] REFERENCES x(id) ...` (keep the `REFERENCES`/`ON DELETE` clauses verbatim).
- Every `tenant_id INTEGER` guard column → `tenant_id TEXT`.
- `author_user_id INTEGER` → `author_user_id TEXT`.
- Indexes on these columns stay as-is (they index whatever type the column now is).
- `tenant/00004_catalogue_tenant.sql`: `ADD COLUMN tenant_id INTEGER NOT NULL DEFAULT 0` → `ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''`.
- **Leave the scs `sessions` table untouched** (`token TEXT PRIMARY KEY`).

- [ ] **Step 1:** Edit `control/00001_control.sql` (tenants, users, invites, audit_log).
- [ ] **Step 2:** Edit `tenant/00001_tenant.sql` (payers, clients, business_profile, custom_items, tax_rates, invoices, work_sessions, line_items, estimates, estimate_line_items, payments, recurring_templates).
- [ ] **Step 3:** Edit `tenant/00002_audit_log.sql`, `tenant/00003_catalogue.sql`, `tenant/00004_catalogue_tenant.sql`.
- [ ] **Step 4: Sanity-check the schema applies** — write a throwaway and run it, or rely on Phase 5's migrate-on-startup. Minimum check now:

```bash
grep -rn "INTEGER PRIMARY KEY\|_id *INTEGER\|uuid *TEXT" internal/db/migrations/
```
Expected: no `INTEGER PRIMARY KEY`, no `*_id INTEGER`, no leftover `uuid TEXT` columns (the scs `sessions` table has neither, so it won't appear).

- [ ] **Step 5: Commit** (build is red from here — that's expected)

```bash
git add internal/db/migrations/
git commit -m "refactor(db): TEXT uuid primary keys and foreign keys (no int PKs)"
```

### Task 2: Update queries + regenerate gen

**Files:** `internal/db/queries/*.sql`, then regen `internal/db/gen`.

- [ ] **Step 1:** Scan `internal/db/queries/*.sql` for anything that selected, inserted, or filtered the now-removed `uuid` column or assumed an int `id`. Common edits:
  - Queries returning `..., uuid, ...` drop the `uuid` column (it no longer exists; `id` carries it).
  - `INSERT ... (uuid, ...) VALUES (?, ...)` lose the separate uuid param — `id` is the supplied uuid.
  - `WHERE uuid = ?` → `WHERE id = ?`.
  - `RETURNING id, uuid, ...` → `RETURNING id, ...`.
- [ ] **Step 2: Regenerate**

```bash
"$(go env GOPATH)/bin/sqlc" generate
```
Expected: `internal/db/gen/models.go` now has `ID string` (no `Uuid` field) and FK fields as `string`/`sql.NullString`.

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries/ internal/db/gen/
git commit -m "refactor(db): regenerate sqlc for uuid string ids"
```

---

## Phase 2 — Platform packages

### Task 3: reqctx int64 → string

**Files:** `internal/reqctx/reqctx.go`, `internal/reqctx/*_test.go`.

- [ ] **Step 1:** Change signatures: `WithTenant(ctx, tenantID string)`, `TenantFrom(ctx) (string, bool)` (absent → `"", false`), `MustTenant(ctx) string`, and the same for `WithUser`/`UserFrom`. The `v.(int64)` type assertions become `v.(string)`; zero-value sentinels become `""`.
- [ ] **Step 2:** Update `reqctx/*_test.go` to pass/expect string ids.
- [ ] **Step 3: Commit** `refactor(reqctx): tenant/user ids are uuid strings`.

### Task 4: audit EntityID → string

**Files:** `internal/audit/audit.go`, `internal/audit/*_test.go`.

- [ ] **Step 1:** `Entry.EntityID int64 → string`. In `Log`, the value is already written via `?` binding — just the field type changes. `tenant`/`user` come from reqctx (now strings).
- [ ] **Step 2:** Update `audit/*_test.go` (`EntityID: 1` → `EntityID: "..."`, any uuid string).
- [ ] **Step 3: Commit** `refactor(audit): EntityID is a uuid string`.

### Task 5: httpx — drop ParseID, session strings

**Files:** `internal/httpx/*.go`, `internal/app/auth_handlers.go`, `internal/app/server.go`, middleware in `internal/httpx`.

- [ ] **Step 1:** Remove `ParseID` (the int path). Keep `ParseUUID`.
- [ ] **Step 2:** Session storage: `sm.Put(ctx, "userID", <uuid string>)` and `"tenantID"` likewise; middleware reads `sm.GetString(...)`. `RequireAuth` maps the session user uuid to a user row by id (no int conversion).
- [ ] **Step 3:** `reqctx.WithTenant/WithUser` calls now pass strings.
- [ ] **Step 4: Commit** `refactor(httpx): uuid-string session ids, remove ParseID`.

---

## Phase 3 — Domain slices (the bulk)

Each slice gets the **same transformation**. Worked example = `client`; apply the identical pattern to the rest.

### Task 6: client slice (worked reference)

**Files:** `internal/client/repository.go`, `service.go`, `handler.go`, `*_test.go`.

The transformation:
- The model struct: `ID int64 json:"-"` + `UUID string json:"id"` → **one** `ID string json:"id"`. Delete the `UUID` field; everywhere it was read, use `ID`.
- Repo method signatures: every `tenantID int64`, `id int64`, `ids []int64` → `string` / `[]string`.
- **Delete the resolve-to-int helpers:** `GetByID(ctx, tenantID, id int64)`, `resolvePayer` (returned `sql.NullInt64`) now returns `sql.NullString` (or just the uuid string), `ResolveClientIDs` (uuid→int) is **deleted** — callers pass uuids straight through.
- `Get(ctx, tenantID, uuid string)` keys on `WHERE id = ?` directly.
- `Create`: the new row id is `ids.New()`; insert it as `id` (no separate uuid). `audit.Entry{EntityID: c.ID}` is already the uuid.
- `BulkDelete(ctx, tenantID, ids []int64)` → `[]string`.
- Handler: `ParseUUID(r, "clientUUID")` result is passed directly to the repo as the id (no `GetByUUID` round-trip to find an int).

- [ ] **Step 1:** Convert `repository.go` (struct, all method sigs, delete resolve helpers, `ids.New()` on create).
- [ ] **Step 2:** Convert `service.go` (signatures `int64 → string`, drop any resolve calls).
- [ ] **Step 3:** Convert `handler.go` (pass uuid directly).
- [ ] **Step 4:** Convert `client/*_test.go` seeds/asserts to string ids.
- [ ] **Step 5: Build just this package's deps far enough to eyeball** — full `go build ./...` is still red until all slices land; review the diff for residual `int64`/`.UUID`/`GetByID`.
- [ ] **Step 6: Commit** `refactor(client): uuid-string ids, drop int resolution`.

### Tasks 7–17: remaining slices

Apply the Task 6 pattern to each. One commit per slice (`refactor(<slice>): uuid-string ids`). Per-slice notes:

- [ ] **Task 7: payer** — referenced by clients/invoices/estimates/recurring via `payer_id`; nullable FK → `sql.NullString`.
- [ ] **Task 8: invoice (incl. payment)** — `client_id`, `payer_id` FKs; `payments.invoice_id`; declares `SessionLinker` (its id params `int64 → string`).
- [ ] **Task 9: estimate** — `client_id`, `payer_id`, `converted_invoice_id`, `estimate_line_items.estimate_id`.
- [ ] **Task 10: recurring** — `client_id`, `payer_id` nullable FKs.
- [ ] **Task 11: session** — table `work_sessions`; `client_id`, `invoice_id`, `author_user_id`; declares `InvoiceChecker` (id params → string).
- [ ] **Task 12: taxrate**.
- [ ] **Task 13: businessprofile** — `tenant_id UNIQUE` 1:1.
- [ ] **Task 14: customitem** — referenced by `line_items.custom_item_id`, `estimate_line_items.custom_item_id`.
- [ ] **Task 15: pricelist** — `price_list_versions`, `items`; `items.price_list_version_id`; `line_items` pins `price_list_version_id` + `item_id` (already uuids in JSON).
- [ ] **Task 16: billing** — `LineItem(Input)`, `ComputeTotals`, `SnapshotBuilder` (reads gen), `LineValidator`: any int id handling → string.
- [ ] **Task 17: smarts + auth + listquery** — `smarts` tools take the slice interfaces (param types follow); `auth` `UsersRepo`/`TenantsRepo` (drop `fillTenantUUID`/`GetByID`-to-int, `Signup` returns a user whose `ID` is the uuid — this also fixes the empty-`tenantId` bug noted in `docs/gotchas.md`); `listquery` id-typed params → string.

### Task 18: app wiring compiles

**Files:** `internal/app/*.go`.

- [ ] **Step 1:** Fix `internal/app` composition + sweep handlers to the new signatures.
- [ ] **Step 2: First full green build of the sweep**

```bash
go build ./... 2>&1 | tail -20
```
Iterate until it builds. Expected end state: exit 0.

- [ ] **Step 3: Commit** `refactor(app): wire uuid-string ids across slices`.

---

## Phase 4 — Tests

### Task 19: green the test suite

**Files:** `internal/app/*_test.go`, every slice `*_test.go`, `internal/audit`, `internal/reqctx` (done), `internal/smarts` fakes.

- [ ] **Step 1:** Update seed helpers (`seedTenantOwner` etc.) to mint/return uuid strings; replace int literal ids (`EntityID: 1`, `tenantID := int64(1)`, `clientID := created.ID`) with uuid strings or the created row's string `ID`.
- [ ] **Step 2: Run the gate**

```bash
go test -race ./... 2>&1 | tail -30
```
Iterate until green.

- [ ] **Step 3: Commit** `test: uuid-string ids across the suite`.

---

## Phase 5 — Verification gate

### Task 20: full gate + e2e

- [ ] **Step 1: Backend gate**

```bash
go vet ./... && test -z "$(gofmt -l .)" && go test -race ./...
CGO_ENABLED=0 go build ./cmd/tallyo && echo CGO_FREE_OK
```
Expected: all clean, binary builds.

- [ ] **Step 2: Migrations apply on a fresh DB** — start the binary against a temp data dir; it runs both goose sequences on startup.

```bash
DATA=$(mktemp -d); go run ./cmd/tallyo --data-dir "$DATA" --port 8097 &
sleep 2; curl -s -o /dev/null -w "%{http_code}\n" localhost:8097/api/auth/session; kill %1
```
Expected: server starts (no migrate error in log), endpoint responds.

- [ ] **Step 3: Frontend check** — `cd web && npm run check && npm run build` → 0/0, build emits.

- [ ] **Step 4: e2e (API unchanged, should pass untouched)**

```bash
task test:e2e               # smoke
SMARTS_E2E=1 task test:e2e  # live Smarts
```
Expected: both green. If a JSON `id` shape changed unexpectedly, fix the slice — the API contract must be byte-for-byte the same.

- [ ] **Step 5: Commit** any test/e2e fixups.

---

## Phase 6 — Docs

### Task 21: rewrite the convention

**Files:** `CLAUDE.md`, `docs/data-model.md`, `docs/gotchas.md`.

- [ ] **Step 1:** Rewrite CLAUDE.md "UUID addressing" — the uuid IS the id; no int-PK-internal split; ids minted via `internal/ids.New()` (uuidv7). Remove mentions of `uuid → row` boundary resolution and the int PK being "internal-only".
- [ ] **Step 2:** Update `docs/data-model.md` ERD to TEXT uuid PKs/FKs.
- [ ] **Step 3:** Update `docs/gotchas.md` — the signup `tenantId`-empty entry is now obsolete (the resolution layer is gone; `Signup` returns the uuid id directly). Remove or rewrite it.
- [ ] **Step 4: Commit** `docs: single uuid-id convention`.

---

## Definition of Done

- `grep -rn "uuid.NewString" internal/` → empty; all ids via `ids.New()`.
- No `INTEGER PRIMARY KEY` / `*_id INTEGER` in `internal/db/migrations/`.
- `go test -race ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build ./cmd/tallyo` all clean.
- `cd web && npm run check && npm run build` clean.
- `task test:e2e` and `SMARTS_E2E=1 task test:e2e` green — JSON API unchanged.
- CLAUDE.md / data-model.md / gotchas.md reflect the single-uuid convention.
