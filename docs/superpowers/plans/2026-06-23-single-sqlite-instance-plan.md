# Plan: collapse to a single SQLite instance

Spec: `docs/superpowers/specs/2026-06-23-single-sqlite-instance-design.md`

Mostly deletion. The `db.Executor` interface + explicit `tenant_id` params mean
repos/services are unchanged — only the composition root and the routing layer move.

## Step 1 — App composition root (`internal/app/app.go`)

Replace the control-DB + registry block (~lines 141–158):

```go
database, err := appdb.Open(filepath.Join(dir, "tallyo.db"))
if err != nil {
    return fmt.Errorf("open db: %w", err)
}
if err := appdb.Migrate(database); err != nil {
    return fmt.Errorf("migrate: %w", err)
}
defer func() {
    if cerr := database.Close(); cerr != nil {
        logger.Error("close db failed", slog.Any("error", cerr))
    }
}()
```

Then `s/control/database/` and `s/tdb/database/` for every repo/service
constructor (auth + all tenant services share one handle). `provisionProfile(reg)`
→ `provisionProfile(database)`. Remove the `tenantdb` import.

## Step 2 — Provisioner (`internal/app/provision.go`)

```go
func provisionProfile(database *sql.DB) auth.ProfileProvisioner {
    return func(ctx context.Context, tenantID int64, in auth.SignupInput) error {
        return auth.ProvisionBusinessProfile(ctx, database, tenantID, in)
    }
}
```

Drop the `tenantdb` import; add `database/sql`.

## Step 3 — Delete the routing layer

`rm -r internal/tenantdb/` (registry.go, conn.go, registry_test.go).

## Step 4 — Comment cleanup

- `internal/db/migrate.go`: `Migrate` is the production path now — retitle its
  doc comment; drop the "Production uses MigrateControl + MigrateTenant" line.
- `internal/db/executor.go`: drop the `tenantdb.Conn` sentence; `*sql.DB` only.
- `internal/audit/audit.go`: same — `txBeginner`/`Execer` are satisfied by
  `*sql.DB`/`*sql.Tx`.

## Step 5 — Docs

`CLAUDE.md` (Database section, Project Layout bullet for `internal/db`/tenantdb)
and `docs/data-model.md`: replace DB-per-tenant wording with single-file +
logical tenancy via `tenant_id`.

## Step 6 — Verify

```
gofmt -l .
go vet ./...
CGO_ENABLED=0 go build ./cmd/tallyo
go test ./... -race
```

Sweep (`internal/app/sweep.go`) is unchanged — it still iterates active tenants
and runs each under its own `reqctx.WithTenant` context; the repos just resolve
to the one shared DB now.
