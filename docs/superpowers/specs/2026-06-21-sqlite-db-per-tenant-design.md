# SQLite DB per Tenant — Design

**Date:** 2026-06-21
**Status:** Approved (design), pending implementation plan

## Problem

Tallyo runs all tenants in a single shared `tallyo-go.db`. Isolation is
enforced only by a `tenant_id` column + WHERE filters on every query. We want
to move to **one SQLite file per tenant**, driven by three goals:

1. **Hard data isolation** — a tenant's rows never share a file with another;
   a query bug can't leak across tenants.
2. **Per-tenant ops** — backup, export, delete, and move a tenant as a single
   file (cheap "delete my org", trivial per-tenant restore).
3. **Scale / write contention** — single SQLite file = one writer; per-tenant
   files give parallel writers and less `busy_timeout` contention as tenant
   count grows (target: **hundreds** of tenants per deployment).

Compliance/residency is **not** a driver, so all DB files may live under one
data dir.

## Key finding that makes this clean

The current schema already supports the split with near-zero query rewrites:

- Only **two** JOINs cross any table boundary, and **both sides are control
  tables**: `users ⋈ tenants` and `support_item_prices ⋈ support_items`.
- **No query joins a tenant table against a control table.** `line_items`
  already **snapshots** `code / description / unit / unit_price` per line and
  pins `catalog_version_id`, so catalogue display needs no live join.

Therefore `sqlc`'s generated `gen` package stays a **single package** — no
split, no join rewrites. Each repo simply runs its existing queries against the
correct `*sql.DB`.

## Topology

```
data/
  control.db                 # global + shared reference data
  tenants/
    tenant-<uuid>.db         # one file per tenant
```

**control.db** (global, single file):
`tenants, users, invites, sessions, catalog_versions, support_items,
support_item_prices`, plus a small `audit_log` for global admin actions
(catalogue upload, tenant create/suspend).

**tenant-<uuid>.db** (one per tenant):
`business_profile, plan_managers, participants, custom_items, tax_rates,
invoices, line_items, estimates, estimate_line_items, payments,
recurring_templates, shifts, audit_log`.

Rationale for users/invites/sessions in control: login is global
(`POST /api/auth/login`, no tenant in URL) and must resolve email → tenant
before any tenant DB is known. Sessions (scs) want a single `*sql.DB`.

## Schema changes (minimal diff)

Tenant tables stay otherwise identical — the only change is removing FK
constraints that point at tables **not present in the tenant file** (with
`foreign_keys=ON`, such an FK errors on insert):

- `tenant_id INTEGER NOT NULL REFERENCES tenants(id)` → **plain column, FK
  dropped**. The column is kept (the file already scopes the tenant, but
  keeping the column means **zero query rewrites** — all `WHERE tenant_id = ?`
  filters still work as a belt-and-suspenders guard).
- `line_items.support_item_id REFERENCES support_items(id)` and
  `line_items.catalog_version_id REFERENCES catalog_versions(id)` → store the
  **UUID** (control-DB integer ids are meaningless across files), FK dropped,
  existence validated in app at write time. Display fields are already
  snapshotted on the line.
- Same-DB FKs are **kept**: `invoice_id`, `estimate_id`, `custom_item_id`,
  `participant_id`, `plan_manager_id`, etc. all reference tenant-local tables.

`audit_log.tenant_id` in the tenant file is redundant but kept (zero-diff).

## Components

### 1. Connection registry — new `internal/tenantdb`

```go
type Registry struct {
    control *sql.DB
    dataDir string
    mu      sync.Mutex
    open    map[string]*entry // uuid -> {db *sql.DB, lastUsed time}
}

func (r *Registry) Control() *sql.DB
func (r *Registry) ForTenant(ctx context.Context) (*sql.DB, error)
```

- `ForTenant` reads the tenant UUID from `reqctx`, returns the cached handle, or
  on miss opens `tenants/tenant-<uuid>.db` via `db.Open`, runs lazy
  `goose.Up` **once per process** (tracked by an in-memory "migrated" set), and
  inserts it into a bounded LRU (cap ~100).
- Over cap → close the least-recently-used **idle** entry. An idle TTL ensures
  a handle is never closed while a request is mid-flight.
- Per-tenant pool kept small: `SetMaxOpenConns(4)` (each tenant is low-traffic;
  100 handles × 4 = 400 fds, well within limits).
- `// ponytail: LRU + idle-TTL, cap 100. For thousands of tenants add
  open-file-limit tuning + smaller pools.`

`db.Open` (existing `internal/db/sqlite.go`) is reused unchanged: same WAL +
`foreign_keys(1)` + `busy_timeout(5000)` + `_txlock=immediate` pragmas.

### 2. Migrations split

Two embedded goose dirs, each with its own `goose_db_version` table:

- `internal/db/migrations/control/*.sql` — tenants, users, invites, sessions,
  catalogue (incl. the 485 KB `00006` catalogue seed — runs once).
- `internal/db/migrations/tenant/*.sql` — the business tables.

`Migrate(control)` runs at startup. Tenant DBs migrate lazily on first open via
the registry. `sqlc` is pointed at **both** schema dirs for type generation
only; runtime DB selection is the repo's job.

### 3. Repo / service DB injection (the bulk of the work)

- **Control-plane repos** (`auth.Users`, `auth.Tenants`, `auth.Invites`,
  session manager): constructed with `reg.Control()` — behaviour unchanged.
- **Tenant-plane repos** (~13): hold `*tenantdb.Registry` instead of `*sql.DB`.
  Each method begins with:
  ```go
  db, err := r.reg.ForTenant(ctx)
  if err != nil { return ... }
  ```
  then runs the existing `gen.New(db)` / `db.BeginTx(ctx)` code unchanged.
- Service constructors take `reg` instead of `conn`; bodies barely change.
- `internal/app` composition root builds one `Registry` and wires it everywhere.

### 4. Tenant provisioning (signup)

No longer a single atomic tx (it spans two files). Ordered with rollback:

1. **control tx:** insert `tenants` row + owner `users` row.
2. create + migrate `tenants/tenant-<uuid>.db`.
3. **tenant tx:** insert `business_profile`.

On failure at any step, unwind the prior steps (delete the tenant file, delete
the control rows). A **startup orphan-sweep** reconciles half-provisioned
tenants (tenant row with no usable file, or file with no row).

### 5. Sweeps, hub, sessions, audit

- **Sweeps** (`internal/app/sweep.go`): already per-tenant. Read
  `ActiveTenantIDs` from control, call `ForTenant` per tenant, run the existing
  overdue/recurring logic against the tenant DB. Only the DB source changes.
- **Realtime hub:** unchanged — global singleton, routed by `Event.TenantID`.
- **Sessions:** scs `sqlite3store` on `control.db`. Behaviour unchanged.
- **Audit:** tenant mutations write `audit_log` in the tenant DB (`audit.WithTx`
  is tx-scoped and rides the tenant tx). Global admin actions write the control
  `audit_log`.

### 6. Per-tenant ops (the payoff)

- **Delete tenant:** mark control `tenants.status`, then delete
  `tenant-<uuid>.db` (+ `-wal`, `-shm`).
- **Export/backup:** `VACUUM INTO` (or SQLite backup API) for a consistent copy
  under WAL.
- **Move:** ship the file; the target host's `control.db` already holds the
  catalogue, so the file is self-sufficient for its business data.

## Non-goals / out of scope

- **No existing-data migration.** Clean cutover (per CLAUDE.md clean-break data
  model): the old single `tallyo-go.db` is discarded; deployments start fresh
  with `control.db` + per-tenant files. No split script.
- No sqlc package split, no query-join rewrites (see Key finding).
- No change to the realtime hub design, the auth/login flow, or the frontend.
- No compliance/residency placement features.

## Risks / decisions

- **LRU eviction vs in-flight requests:** mitigated by idle-TTL — only idle
  handles are closed. Cap (100) is generous for "hundreds of tenants".
- **Cross-file provisioning is not atomic:** mitigated by ordered rollback +
  startup orphan-sweep.
- **Dropped catalogue FK:** existence now validated in app at write time;
  display data already snapshotted on `line_items`, so read paths are
  unaffected.
- **fd usage:** 100 handles × 4 conns = ~400 fds; fine. Revisit at thousands.

## Testing

- `tenantdb.Registry`: open/cache/evict/reopen, lazy migrate-once,
  idle-TTL-not-closing-in-flight.
- Provisioning: success path + each failure step rolls back cleanly; orphan
  sweep reconciles.
- Isolation: a tenant repo method run under tenant A's ctx never sees tenant B's
  rows (separate files).
- Sweeps: multi-tenant overdue/recurring across separate files.
- Existing per-slice repo tests re-pointed through the registry.
- Gates: `go test ./... -race`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./cmd/tallyo`.
