# Collapse DB-per-tenant → single SQLite instance

**Date:** 2026-06-23
**Status:** Approved (best-guess, fast-track per user)

## Goal

Run the whole app off **one** SQLite file instead of a control DB plus one file
per tenant. Motivation: **ops simplicity** — one file to back up, migrate, and
debug; delete the registry/LRU/lazy-migrate routing layer.

## Decision

- **Tenancy stays logical.** Keep every `tenant_id` column and every
  `WHERE tenant_id = ?` guard. Multi-tenant in the schema, single file on disk.
  `reqctx` still carries the tenant id (audit stamping + query guards). The
  `ResolveTenant` middleware and per-tenant sweep are unchanged.
- This is a **clean break** — no data migration from existing per-tenant files
  (consistent with the project's clean-break data model).

## Why it's small

The codebase already abstracts the DB behind `db.Executor`, and every tenant
repo takes `tenant_id` as an explicit parameter. A combined `Migrate(conn)` that
applies both goose sequences to one file already exists (tests use it). So the
per-tenant `*sql.DB` was only ever reached through a thin routing handle
(`tenantdb.Conn`) — collapsing means handing repos the **same** `*sql.DB` the
control repos already use, then deleting the routing layer.

## Changes

1. **`internal/app/app.go`** — open one DB (`<data-dir>/tallyo.db`), run
   `appdb.Migrate(db)` (both sequences), pass that single `*sql.DB` to **both**
   control repos and tenant services. Drop `tenantdb.New`, `reg.Tenant()`,
   `reg.Close()` → `db.Close()`.
2. **`internal/app/provision.go`** — `provisionProfile` takes `*sql.DB` directly
   instead of the registry; no per-tenant open.
3. **Delete `internal/tenantdb/`** — `registry.go`, `conn.go`, `registry_test.go`.
4. **`internal/db/migrate.go`** — `Migrate` is now the production path; tidy the
   comment. `MigrateControl`/`MigrateTenant` stay (composed by `Migrate`).
5. **`internal/db/executor.go`, `internal/audit/audit.go`** — comments drop the
   `tenantdb.Conn` mention; `*sql.DB` is the only implementation now.
6. **Docs** — `CLAUDE.md` Database section + `docs/data-model.md`: replace the
   DB-per-tenant description with single-file, logical-tenancy.

## Migrations & audit_log

`Migrate` runs the control sequence then the tenant sequence on one file.
`control/00001` creates `audit_log` **with** FKs to `tenants(id)`/`users(id)`;
`tenant/00002` is `CREATE TABLE IF NOT EXISTS audit_log` and no-ops. Result: one
`audit_log` with real FKs — valid now that tenants/users live in the same file.
sqlc already reads the control `audit_log` for types, so no regen needed (schema
unchanged). Tests already exercise this combined mode.

## Out of scope

- Removing `tenant_id` columns / the tenants table (chose "keep tenant_id").
- Merging the two goose sequences into one (no benefit; more churn).
- Any change to query SQL, sqlc gen, or the HTTP/UUID API.

## Verification

`go build ./cmd/tallyo` (CGO off), `go test ./... -race`, `go vet`, `gofmt -l`.
Manual: signup → profile created; create invoice; overdue sweep runs.
