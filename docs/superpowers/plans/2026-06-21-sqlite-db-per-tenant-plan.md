# Implementation Plan — SQLite DB per Tenant

Executes the approved design:
`docs/superpowers/specs/2026-06-21-sqlite-db-per-tenant-design.md`.
Read that spec first — it is the source of truth for topology, schema diff, the
connection registry, provisioning, and the cross-DB references table.

## Gates (run after EVERY phase — all must pass before moving on)

```bash
gofmt -l .                                   # must print nothing
go vet ./...
go test ./... -race
CGO_ENABLED=0 go build ./cmd/tallyo
"$(go env GOPATH)/bin/sqlc" generate          # then: git diff --exit-code internal/db/gen
cd web && npm run check && cd ..             # only if web/ touched
```

Commit at the end of each phase (Conventional Commits). Keep the working tree
green between phases.

## Phase 0 — Verify the load-bearing assumptions

- Re-run the cross-boundary JOIN grep; confirm only `support_item_prices ⋈
  support_items` and `users ⋈ tenants` exist (both control↔control):
  `grep -rniE "join (support_items|support_item_prices|catalog_versions|users|tenants)" internal/db/queries/*.sql`
- Confirm `sqlc.yaml` (v2) accepts a **list** of schema dirs into one `out`
  package. Prove it with a throwaway two-dir config before committing to the
  layout. If unsupported, fall back: keep one schema dir for sqlc type-gen, and
  let goose own the physical per-dir DDL split.
- No code change this phase; record findings in the commit message.

## Phase 1 — Split migrations into control/ and tenant/

- New dirs `internal/db/migrations/control/` and `internal/db/migrations/tenant/`.
- **`00001_ndis_baseline.sql` is SPLIT, not moved** — it defines both control
  and tenant tables. Control half: `tenants, users, invites, sessions,
  catalog_versions, support_items, support_item_prices`, control `audit_log`
  (FKs intact). Tenant half: the ~13 business tables.
- Move the 485 KB `00006` catalogue seed to `control/`.
- In the **tenant** DDL, drop cross-DB FK constraints (keep the columns):
  - `tenant_id` → plain column (FK to tenants dropped)
  - `line_items` / `estimate_line_items` `support_item_id`, `catalog_version_id`
    → change type to **TEXT (UUID)**, FK dropped. (Real type change — see Phase 3.)
  - `audit_log` `tenant_id` + `user_id`, `shifts.author_user_id`, and any other
    `*_user_id REFERENCES users` → FK dropped, column kept.
- Keep same-file FKs: `invoice_id`, `estimate_id`, `custom_item_id`,
  `participant_id`, `plan_manager_id`, `shift_id`.
- Carry `idx_audit_entity` / `idx_audit_batch` into the tenant DDL.
- Each dir gets its own goose sequence + `goose_db_version` table.

## Phase 2 — Migration runner split

- `internal/db/migrate.go`: `MigrateControl(db)` and `MigrateTenant(db)` over the
  two embedded dirs.
- Startup runs `MigrateControl(control)`. Tenant DBs migrate lazily (Phase 4).

## Phase 3 — sqlc regenerate + fix UUID type change

- Point `sqlc.yaml` at both schema dirs (per Phase 0). `sqlc generate`.
- `support_item_id` / `catalog_version_id` params flip `int64/NullInt64` →
  `string/NullString` on line_items + estimate_line_items insert/update. Fix the
  repo/service call sites that populate them to pass UUIDs. Add app-side
  validation that the referenced catalogue UUID exists in control.db at write.
- Gates green.

## Phase 4 — Connection registry `internal/tenantdb`

- `Registry{ control *sql.DB; dataDir string; mu; open map[int64]*entry }`.
- `Control() *sql.DB`, `ForTenant(ctx) (*sql.DB,error)` (reads
  `reqctx.TenantFrom`), `ForTenantID(id) (*sql.DB,error)`.
- Open via existing `db.Open`, file `tenants/tenant-<id>.db`. Lazy
  `MigrateTenant` once per process (in-memory migrated set). LRU cap 100,
  idle-TTL 5m eviction + 1m background sweep, `SetMaxOpenConns(4)`.
- Tests: open/cache/evict/reopen, migrate-once, idle handle not closed in-flight.

## Phase 5 — Re-point repos & services

- Control repos (`auth.Users/Tenants/Invites`, session mgr): construct with
  `reg.Control()`.
- Tenant repos (~13): hold `*tenantdb.Registry`; each method tops with
  `db, err := r.reg.ForTenant(ctx)` then existing `gen.New(db)`/`BeginTx`.
- Service constructors take `reg` instead of `conn`. Wire one `Registry` in
  `internal/app`.
- Re-point existing per-slice repo tests through the registry.

## Phase 6 — Provisioning + orphan sweep

- Signup: ordered cross-DB create — control tx (tenants+owner user) → create &
  migrate tenant file → tenant tx (business_profile). Roll back prior steps on
  failure (delete file, delete control rows).
- Startup orphan-sweep reconciles half-provisioned tenants.

## Phase 7 — Sweeps + clean cutover

- `internal/app/sweep.go`: read `ActiveTenantIDs` from control, per id
  `reg.ForTenantID(id)` + `reqctx.WithTenant(bg, id)`, run existing logic.
- Remove the old single-`tallyo-go.db` open/migrate path (clean cutover; no data
  migration). Update `--data-dir` docs if needed.
- Realtime hub, sessions: unchanged.

## Phase 8 — Final verification

- Full gate run, `-race`. Manual smoke: signup creates control rows + a
  `tenants/tenant-<id>.db`; a second tenant gets its own file; deleting the file
  removes only that tenant's data.
- Update `docs/data-model.md` to reflect the control/tenant split (CLAUDE.md
  mandates keeping both ERDs in sync).

When every phase is complete and ALL gates pass, print exactly:
`DONE-DBPERTENANT`
