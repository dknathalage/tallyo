# Single UUIDv7 Identifier — Design

**Date:** 2026-06-24
**Status:** Approved (brainstorm)
**Scope:** Replace Tallyo's dual-identifier data model (int64 `id` PK + separate
`uuid` column on every table) with a **single uuid string identifier** per row,
generated as **UUIDv7** through one shared helper. Kill all int64 primary keys
and the `uuid → int` resolution boilerplate. Repo-wide, big-bang rewrite.

## Goal

Today every table carries two identities: an internal `id INTEGER PRIMARY KEY
AUTOINCREMENT` and a public `uuid TEXT`. Handlers resolve `uuid → row` at the
boundary, operate on the int PK internally, and expose the uuid as the JSON `id`.
This is the documented "UUID addressing" convention. The goal is **one id per
row, one id generator, one convention** — the uuid *is* the primary key, used
end to end, and every id in the app is minted the same way (UUIDv7).

This is a readability/consistency change, not a performance one; at this app's
scale (single self-hosted SQLite file) the int-vs-uuid PK choice is immaterial at
runtime. The win is a simpler mental model and the deletion of the resolution
layer.

## Non-Goals

- No behavioural change to any feature, endpoint, or JSON shape. JSON `id` /
  `*Id` fields are already uuid strings; they stay byte-for-byte the same.
- No `WITHOUT ROWID` tables or other storage tuning — default rowid tables with
  a TEXT PK. (Deliberate: no storage cleverness nobody asked for.)
- No change to document numbering (`internal/numbering` mints invoice/estimate
  *numbers* like `INV-0001`, unrelated to row PKs).
- No data migration. The data model is clean-break (CLAUDE.md); there is no
  production data and no upgrade path to preserve.

## Why UUIDv7 (single convention)

Int PKs gave free insertion-order sorting (`ORDER BY id`). Random UUIDv4 PKs do
not. **UUIDv7 is time-ordered**, so `ORDER BY id` keeps yielding chronological
order and list queries need no rewrite. To avoid a *second* convention (v7 for
PKs, v4 elsewhere), **all** id generation moves to UUIDv7 via one helper —
`uuid.NewString()` (v4) is eliminated everywhere. The only trade-off, that a v7
id encodes its creation time, is irrelevant for internal invoice-app ids.

## Architecture

### 1. Schema — merge the two ids, uuid becomes the PK

For every table that has both `id INTEGER PRIMARY KEY AUTOINCREMENT` and a `uuid
TEXT` column: drop the int `id`, drop the separate `uuid` column, and make the
**uuid the primary key** as `id TEXT PRIMARY KEY`. Every foreign key and guard
column that referenced an int id becomes `TEXT`.

Because the schema is clean-break, **edit the existing migration files in place**
(`internal/db/migrations/control/*.sql`, `internal/db/migrations/tenant/*.sql`)
rather than adding new migrations — there is no data or version history to
preserve.

Tables affected:
- **Control:** `tenants`, `users`, `invites` (incl. `created_by` FK),
  `audit_log` (incl. `tenant_id`, `user_id`).
- **Tenant:** `payers`, `clients`, `business_profile`, `custom_items`,
  `tax_rates`, `invoices`, `work_sessions`, `line_items`, `estimates`,
  `estimate_line_items`, `payments`, `recurring_templates`,
  `price_list_versions`, `items`, and the tenant `audit_log`.
- FK / guard columns to convert to TEXT: `tenant_id` (on every tenant table),
  `client_id`, `payer_id`, `invoice_id`, `session_id`, `custom_item_id`,
  `converted_invoice_id`, `estimate_id`, `price_list_version_id`, `item_id`,
  `author_user_id`, `invites.created_by`, `audit_log.user_id`.
- **Unchanged:** the scs `sessions` table — already `token TEXT PRIMARY KEY`.

### 2. One id generator

Add `internal/ids` exposing:

```go
package ids

import "github.com/google/uuid"

// New returns a time-ordered UUIDv7 string — the single id convention.
func New() string { return uuid.Must(uuid.NewV7()).String() }
```

Replace all **41** `uuid.NewString()` (v4) call sites across `internal/` with
`ids.New()`. Definition of done: `grep -rn "uuid.NewString" internal/` returns
zero hits. (`google/uuid` is already a dependency and provides `NewV7`.)

### 3. Platform layer

- `internal/reqctx`: `TenantID` / `UserID` and their `With*`/`*From`/`MustTenant`
  signatures change `int64 → string`.
- `internal/audit`: `Entry.EntityID` changes `int64 → string`; `Log` writes it as
  TEXT.
- Session cookie: store the user and tenant ids as uuid strings
  (`sm.Put(ctx, "userID", <uuid>)`, etc.) instead of `int`. `RequireAuth` /
  `ResolveTenant` middleware read strings (`GetString`).
- `internal/httpx`: remove `ParseID` (int path). `ParseUUID` remains and, because
  the uuid is now the PK, its result is used directly as the row key — no
  secondary lookup to resolve an int.

### 4. Domain slices — net deletion

Current pattern per slice: `ParseUUID(r, "xUUID") → repo.GetXByUUID(uuid) →
operate on row.ID (int)`, with `GetXByUUID` existing solely to turn a uuid into an
int PK, and every model carrying both `ID int64` and `UUID string`.

After: parse the uuid and query by it directly. Each model collapses to a single
`ID string`. The `GetXByUUID`-to-resolve-int helpers and the dual ID/UUID fields
are removed; repositories, services, and handlers key on the string id
throughout. Inbound FK uuids are stored as-is (no resolve-to-int before insert).

Slices: `invoice` (incl. payment), `estimate`, `recurring`, `session`, `client`,
`payer`, `taxrate`, `businessprofile`, `customitem`, `pricelist`, `auth`,
`smarts`, `export`, plus `internal/billing` (snapshot/line-item id handling). The
consumer-declared cross-slice interfaces (`invoice.SessionLinker`,
`session.InvoiceChecker`) change their id params `int64 → string`.

### 5. sqlc regeneration

After editing the migrations, regenerate `internal/db/gen`
(`"$(go env GOPATH)/bin/sqlc" generate`). Generated models lose the `Uuid` field
and flip `ID int64 → ID string`; FK fields flip to `string` / `sql.NullString`.
Hand-written queries in `internal/db/queries/*.sql` that filtered or inserted by
int id now bind strings — mechanical, but every `queries/*.sql` and its callers
must be checked. The `gen` package stays a single package (do not split).

### 6. Documentation

- Rewrite the CLAUDE.md **"UUID addressing"** convention: there is no longer an
  int-PK-internal / uuid-external split — the uuid is the id, everywhere. Note the
  single UUIDv7 generator (`internal/ids`).
- Update `docs/data-model.md` (ERD) to reflect TEXT uuid PKs/FKs.
- Update `docs/gotchas.md` if the signup `tenantId` note changes (the resolution
  layer it references is being removed).

## Data Flow

Unchanged at the API boundary. Internally the path shortens:

```
Before: HTTP {uuid} → ParseUUID → GetXByUUID → int PK → query(int) → row → JSON {id: uuid}
After:  HTTP {uuid} → ParseUUID → query(uuid) → row → JSON {id: uuid}
```

## Error Handling

- A not-found uuid behaves exactly as today (the direct query returns no row →
  404 at the handler), only without the intermediate resolve step.
- FK integrity is still enforced by SQLite (`foreign_keys=ON`); TEXT FKs reference
  TEXT PKs. Inserting an unknown FK uuid fails the same way an unknown int did.
- `audit.WithTx` continues to wrap every mutation; `EntityID` is now a string.

## Testing

This is where most of the work lands. Every `*_test.go` that seeds rows with int
ids or asserts on `row.ID` (e.g. `internal/app/*_test.go`, each slice's
`*_test.go`, `internal/audit/*_test.go`, `internal/reqctx/*_test.go`) churns to
uuid strings. Test seed helpers (`seedTenantOwner`, etc.) return string ids.

Gate (all must pass):
- `go test -race ./...`
- `go vet ./...` and `gofmt -l .` clean
- `CGO_ENABLED=0 go build ./cmd/tallyo` (cgo-free binary builds)
- `cd web && npm run check` (0/0) and `npm run build`
- e2e: `task test:e2e` (smoke) and `SMARTS_E2E=1 task test:e2e` (live Smarts) both
  green — the JSON API is unchanged, so these should pass without edits.

## Risk

The risk is **breadth, not depth**: a wide, mostly-mechanical sweep across ~15
tables, the `gen` package, every domain slice, the platform packages, and every
test. No algorithmic complexity, but many files change together and the codebase
does not compile until the sweep is internally consistent (the int→string flip
must land across migrations, gen, repos, and callers in one coherent pass).
Best executed on a single dedicated branch, compiled and tested as a whole.

## Open Implementation Details (for the plan)

- The exact ordering of the sweep so intermediate commits are reviewable (e.g.
  migrations + gen first, then platform, then slices alphabetically, then tests).
- Whether `internal/ids` is a new package or folded into an existing platform
  package (e.g. `db`).
- Any query currently relying on `last_insert_rowid()` / `RETURNING id` semantics
  that needs adjusting when the PK is a client-supplied uuid.
- `sql.NullInt64` FK fields (nullable FKs like `payer_id`) → `sql.NullString`.
