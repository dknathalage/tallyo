# Postgres migration + cheap multi-environment GCP deployment

**Date:** 2026-06-26
**Status:** Approved (design)

## Summary

Migrate Tallyo's persistence from embedded SQLite (modernc) to Postgres (hard
replace — no coexistence), and stand up a cheap, multi-region/multi-project
capable GCP deployment built with OpenTofu modules orchestrated by Terragrunt.
Initial deployment is a single GCP project in one region, with `dev`, `stg`, and
`prd` environments sharing one Cloud SQL instance via three separate databases.
A `docker-compose` stack provides the local equivalent.

As part of this work the **realtime/SSE subsystem is removed** and replaced with
client-side polling (§2). This makes the server fully stateless — no in-process
connection state — so Cloud Run scales to zero (and horizontally) cleanly with no
`min=1`/`max=1` constraints. It also deletes a meaningful amount of code.

This changes Tallyo's identity from "single self-hosted binary with embedded
SQLite" to a Postgres-backed web service. That trade-off was chosen
deliberately.

## Goals

- Replace SQLite with Postgres across driver, sqlc, goose, and session store.
- Remove the SSE realtime subsystem; the SPA polls instead. Server becomes
  stateless.
- Run on the cheapest viable managed GCP stack (Cloud Run scale-to-zero +
  Cloud SQL shared-core).
- Support multiple regions and projects in the IaC layout without restructuring;
  deploy only one project/region now.
- `dev`/`stg`/`prd` share one Cloud SQL instance, one database each.
- Provide a `docker-compose` for local dev that mirrors the cloud topology.

## Non-goals (YAGNI — add when needed)

- Cloud SQL HA, read replicas, or regional failover.
- VPC connector / private IP (the Cloud Run built-in Cloud SQL unix socket
  suffices).
- CI/CD pipeline definitions (tracked as a possible follow-up, not in this spec).
- Custom domains, external load balancer, Cloud Armor.
- `timestamptz` for the business `created_at`/`updated_at` columns (they stay
  `TEXT`). **Exception:** the scs `sessions.expiry` column *does* become
  `timestamptz` because `postgresstore` requires it (§1.5).
- Migrating money columns to `numeric`. They become `double precision` (§1.3a),
  preserving the existing `float64` contract. `numeric` is a deliberate
  non-goal: it would force `pgtype.Numeric`/string across all `billing` math —
  a separate, larger change. (`ponytail: double precision keeps float64; move to
  numeric only if rounding errors surface in money totals`.)
- Keeping SQLite as an option (explicitly dropped).

---

## 1. Database migration (SQLite → Postgres)

### 1.1 Driver and connection

- Replace `modernc.org/sqlite` with `github.com/jackc/pgx/v5` used through
  `github.com/jackc/pgx/v5/stdlib`, so the code keeps `database/sql` and the
  existing `db.Executor` interface unchanged. No repository or service code
  changes for the connection abstraction.
- Rename `internal/db/sqlite.go` → `internal/db/postgres.go`:
  - `Open(dsn string) (*sql.DB, error)` replaces `Open(path string)`.
  - Drop all SQLite pragmas (`journal_mode(WAL)`, `foreign_keys(1)`,
    `busy_timeout`, `_txlock=immediate`). Postgres provides MVCC, enforced FKs,
    and real `BEGIN` locking natively.
  - Connection pool: keep a small bounded pool (`SetMaxOpenConns`,
    `SetMaxIdleConns`) sized for a single-instance app; set a finite
    `SetConnMaxLifetime` (e.g. 30m) appropriate for Cloud SQL.
- DSN comes from the `DATABASE_URL` environment variable. `DataDir()` and the
  `tallyo.db` file path are removed; `internal/app` reads `DATABASE_URL` instead
  of resolving a data directory and opening a file.
- **Cloud SQL socket DSN form (pinned).** On Cloud Run with
  `--add-cloudsql-instances`, the instance is reached via the unix socket
  `/cloudsql/PROJECT:REGION:INSTANCE/.s.PGSQL.5432` — no public IP, no VPC
  connector. `DATABASE_URL` uses the URL form pgx/stdlib accepts:
  `postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT:REGION:INSTANCE`
  (empty host authority, socket dir in the `host` query param). Local/compose
  uses the ordinary `postgres://user:pass@host:5432/db?sslmode=disable` form.
- Dropping the SQLite pragmas means the `_ "modernc.org/sqlite"` blank import is
  removed and replaced by `_ "github.com/jackc/pgx/v5/stdlib"`. (PG enforces
  same-table FKs natively; tenant isolation remains app-level `WHERE tenant_id =
  $n` guards — Postgres does not and should not enforce tenancy.)

### 1.2 The `_txlock=immediate` race

SQLite used `_txlock=immediate` to make the numbering slice's read-then-insert
(MAX read + INSERT) take the write lock at `BEGIN`, avoiding
`SQLITE_BUSY_SNAPSHOT`. In Postgres this race is handled by the existing unique
constraint + retry already present in `internal/numbering` (no DSN-level lock mode
exists; `SELECT ... FOR UPDATE` is an available fallback but the unique-violation
retry is sufficient).

**Required code change — `isRetryable`.** `internal/numbering/numbering.go`
classifies retryable errors by SQLite error substrings (`"busy"`, `"locked"`,
`"unique"`, `"constraint"`). pgx surfaces errors as `*pgconn.PgError` with
SQLSTATE codes, not those substrings. `isRetryable` is rewritten to match
`pgconn` SQLSTATEs: `23505` (unique_violation) and `40001`
(serialization_failure), via `errors.As(err, *pgconn.PgError)`. This is a
mandatory edit, not "the existing loop is preserved."

The behavioral contract — no duplicate document numbers under concurrency — is
unchanged and must be covered by the existing numbering concurrency test, ported
to Postgres (its test harness builds a table with SQLite-only
`substr`/`CAST`/`LIKE` SQL that must be ported too — see §1.3a/§1.7).

### 1.3 sqlc

- `sqlc.yaml`: `engine: "sqlite"` → `engine: "postgresql"`.
- Every query in `internal/db/queries/*.sql` uses `?` positional placeholders
  (SQLite style). These are mechanically rewritten to Postgres `$1, $2, …`
  numbered placeholders. `RETURNING *` and `ON CONFLICT (...) DO UPDATE` are
  already valid Postgres and stay.
- **Non-mechanical query edits (NOT just placeholders).** The following are
  SQLite-isms that must be rewritten, not just re-placeholdered:
  - `CAST(... AS REAL)` → `CAST(... AS double precision)` —
    `internal/db/queries/payments.sql:5`, `invoices.sql:84`, and any other
    `AS REAL` cast. (`AS INTEGER` casts are valid Postgres and stay.)
  - `date('now')` (SQLite) → Postgres. `invoices.sql:79`
    (`SelectOverdueInvoicesForTenant`, on the live sweep path
    `internal/invoice/status.go`) compares `due_date < date('now')`. `date('now')`
    is invalid in Postgres. `due_date` is `TEXT` (ISO-8601), so preserve the
    string-ordering semantics with `due_date < CURRENT_DATE::text` (equivalently
    `due_date::date < CURRENT_DATE`).
  - The numbering MAX queries `invoices.sql:73` / `estimates.sql:74` use
    `CAST(COALESCE(MAX(CAST(substr(number, CAST(sqlc.arg(prefix_len) AS INTEGER)
    + 1) AS INTEGER)), 0) AS INTEGER)`. Postgres `substr` is 1-indexed like
    SQLite so the offset is unchanged, but the whole expression must be verified
    to compile under the postgresql engine and return an integer (the inner
    `CAST(... AS INTEGER)` on a text suffix is valid in PG).
- The `schema:` list in `sqlc.yaml` continues to point at the migration files
  (control dir + the explicitly-listed tenant files), now interpreted as
  Postgres DDL — which first requires the DDL audit in §1.3a.
- Regenerate `internal/db/gen/` with `sqlc generate`. With `REAL` →
  `double precision` (§1.3a), sqlc continues to map money columns to Go
  `float64`, so the generated Go API stays materially the same and downstream
  slice/billing code is unaffected except where generated types shift.

### 1.3a Migration DDL audit (REQUIRED — the schema does NOT port unchanged)

The migrations contain SQLite-only type affinities that are not all valid or
faithful Postgres. Each must be addressed before §1.3's `sqlc generate`:

- **`BLOB` → `bytea`.** `internal/db/migrations/control/00001_control.sql:51`
  (`sessions.data BLOB`). `BLOB` is not a Postgres type. (This column is replaced
  wholesale by the postgresstore schema — see §1.5.)
- **`REAL` money columns → `double precision`.** Pervasive across the tenant
  migrations (`tax_rates.rate`, `invoices`/`estimates` `subtotal`/`tax`/`total`,
  `line_items` `quantity`/`unit_price`/`line_total`, `payments.amount`,
  `catalogue_items.unit_price`, `recurring_templates.*`, etc.). `REAL` is valid
  PG but is 4-byte float (lossy); `double precision` (8-byte) matches Go
  `float64` and the current SQLite REAL→float64 behavior. All `REAL` →
  `double precision`.
- **`REAL` non-money column.** `sessions.expiry REAL` → `timestamptz` per
  postgresstore (§1.5), not `double precision`.
- **`INTEGER`-as-boolean columns** (`is_current`, `taxable`, any `0/1` flags)
  stay `INTEGER`; existing `WHERE is_current = 1` predicates remain valid in PG,
  and sqlc generates `int32`/`int64` exactly as today. No change required, but
  the audit must confirm no column is silently expected to be `bool`.

The audit is a concrete checklist deliverable: enumerate every column type across
all migration files and confirm each is valid Postgres. The §7 acceptance
criteria depend on it.

### 1.4 goose migrations

- `internal/db/migrate.go`: dialect `sqlite3` → `postgres`.
- The two-sequence model (control + tenant, distinct version tables via
  `SetTableName`) is kept; goose's Postgres dialect supports custom version-table
  names.
- **Verify the two-sequence + shared-`audit_log` interplay under Postgres.** The
  current model relies on the tenant `audit_log` migration using
  `CREATE TABLE IF NOT EXISTS` so it coexists with the control `audit_log` in one
  database (sqlc deliberately omits the tenant audit_log file). Both goose `Up`
  calls run sequentially against the same Postgres database; confirm both version
  tables and the `IF NOT EXISTS` audit_log apply cleanly together (goose takes a
  per-run advisory lock; sequential runs are fine). This is a verification step,
  not a code change.
- DDL portability is handled by the §1.3a audit (`BLOB`/`REAL` are NOT
  no-ops). `TEXT` PKs/columns and same-file FKs do port unchanged; `IF NOT
  EXISTS` is valid in Postgres.
- Migrations still run on app startup (`appdb.Migrate(db)`), now against the
  environment's Postgres database. One database per environment means each env's
  startup migrates its own database independently.

### 1.5 Session store

- Replace `alexedwards/scs/sqlite3store` with `alexedwards/scs/postgresstore`.
- **The `sessions` table schema changes and must be authored explicitly.**
  `postgresstore.New(db)` does NOT create its table — the caller must. The
  current SQLite `sessions(token TEXT PK, data BLOB, expiry REAL)` is replaced by
  the postgresstore-required schema:
  ```sql
  CREATE TABLE sessions (
      token  text PRIMARY KEY,
      data   bytea NOT NULL,
      expiry timestamptz NOT NULL
  );
  CREATE INDEX sessions_expiry_idx ON sessions (expiry);
  ```
  This replaces the `sessions` table DDL in `control/00001` (clean-break schema —
  no data migration, so editing the baseline migration in place is acceptable).
- Session semantics (cookie-backed, SQLite-backed → Postgres-backed) are
  otherwise unchanged.

### 1.6 Tenancy model (unchanged)

Logical tenancy is preserved exactly: one database holds all tenants' rows, every
business row carries `tenant_id`, every query guards `WHERE tenant_id = ?` (now
`= $n`). Environments are isolated at the *database* level (three databases), not
via Postgres schemas and not via separate instances.

### 1.7 Tests

- Go tests that open SQLite via `appdb.Open(tmpfile)` must target Postgres.
  Approach: run tests against a disposable Postgres (the compose Postgres, a
  testcontainer, or a `TEST_DATABASE_URL`). The plan picks one; the simplest
  viable option that keeps `go test ./...` runnable locally and in CI is
  preferred (e.g. a `TEST_DATABASE_URL` that defaults to the compose instance,
  skipping DB-touching tests when unset).
- The numbering concurrency test (§1.2) must pass against Postgres.

---

## 2. Statelessness: remove SSE, poll instead

The only things tying the app to a single, always-on instance are two in-process
mechanisms: the in-process SSE hub (`internal/realtime`) and the hourly sweep
ticker (`internal/app/sweep.go`). The SSE hub is removed entirely (the SPA polls);
the sweep clock moves outside the process. The result is a stateless server that
scales to zero and horizontally with no instance-count constraints.

### 2.1 Remove the realtime/SSE subsystem (backend)

Delete the push-event machinery; nothing replaces it server-side.

- **Delete `internal/realtime/`** — the hub, the `/api/events` SSE handler, and
  their tests (`hub.go`, `events_handler.go`, `hub_test.go`,
  `events_handler_test.go`).
- **Delete `internal/events/`** — the `events.Notifier` package.
- **Remove the broadcast calls from every service.** Eight service files
  construct/use a notifier and call `Created/Updated/Deleted`:
  `internal/{catalogue,client,estimate,invoice,payer,recurring,taxrate}/service.go`
  and `internal/session/{service.go,service_items.go}`. Drop the notifier field,
  constructor parameter, and the `s.events.*(ctx, id)` calls. The post-commit
  broadcast simply goes away; the audit log (`audit.WithTx`) is unaffected.
- **`internal/app`:** remove the `Events *realtime.EventsHandler` field and the
  `pr.Get("/events", …)` route in `server.go`; remove hub construction/wiring in
  `app.go`; update the app tests that build `realtime.NewHub()`
  (`clients_test.go`, `events_test.go` → delete, `catalogue_import_test.go`).
- This is a net code deletion. The CLAUDE.md convention "broadcasts an SSE event
  from the service after commit" is removed/updated as part of the change.

### 2.2 Client polling (frontend)

Replace push subscriptions with polling. The existing client already resyncs by
refetching, so this is a mechanism swap, not a behavior rethink.

- **Delete `web/src/lib/realtime/events.ts`** and the `openEvents()/closeEvents()`
  calls in `web/src/routes/[tenant]/+layout.svelte`.
- Add a small polling helper (e.g. `web/src/lib/realtime/poll.ts`, ~20 lines):
  given a refetch callback, it refetches (a) on mount, (b) on an interval
  (default ~30s), and (c) on `visibilitychange`/window focus; returns a cleanup
  that clears the interval + listener. (`ponytail: fixed 30s interval + focus
  refetch; tune the interval only if it feels stale or chatty`.)
- Replace the `onEntity(entity, cb)` subscriptions in the three stores
  (`stores/sessions.svelte.ts`, `stores/collection.svelte.ts` — the generic CRUD
  store, covering most entities — and `stores/businessProfile.svelte.ts`) with the
  poll helper bound to the same refetch each `onEntity` used to call.
- SvelteKit `load` functions already refetch on navigation, so polling mainly
  covers "data changed while the user sits on a list." For a single-org app a 30s
  interval + focus refetch is ample; no websocket/SSE, no live cursor.

### 2.3 Sweep: external clock (the one remaining background concern)

A stateless scale-to-zero server cannot self-trigger hourly cron — no instance is
guaranteed up to hold a ticker. This is not statefulness; cron simply lives
outside disposable compute.

- Extract the per-tenant sweep from `internal/app/sweep.go` into a plain callable
  function (no HTTP/ticker dependency) so any trigger can run it.
- Expose it behind `POST /api/internal/sweep`, mounted **outside** the
  cookie-`RequireAuth` `/api` group (machine endpoint, no session).
- **Cloud Scheduler** runs an hourly job per environment that calls this endpoint
  with an **OIDC token** (Scheduler SA → Cloud Run, `roles/run.invoker`).
- **Auth — two gates:** (1) Cloud Run IAM validates the OIDC invocation before the
  request reaches the app (primary gate); (2) the handler also checks a required
  shared-secret header (value from Secret Manager) as defense-in-depth and as the
  sole gate in local/compose where there is no Cloud Run IAM.
- The in-process ticker is retained **only for local/compose**, gated by an env
  flag (`SWEEP_TICKER=1`, off by default). In cloud, Scheduler is the sole driver.
- The sweep is idempotent (overdue/recurring re-evaluation is safe to run twice),
  so a duplicate trigger during a deploy overlap is harmless.

### 2.4 Cloud Run instance configuration

- `min-instances=0` (true scale-to-zero — chosen for cost).
- **No `max-instances` pin** is required for correctness now that the server is
  stateless; a modest cap (e.g. `max-instances=3`) is fine purely as a cost
  guardrail for a single-org app. Any instance serves any request; deploy-time
  overlap is harmless (no shared in-process state, sweep idempotent).
- `concurrency=80`; Cloud Run gen2 execution environment.

---

## 3. GCP architecture (cheap, single project now)

- **Compute:** Cloud Run service per environment (`dev`, `stg`, `prd`) in one
  project/region. Each connects to Cloud SQL through the built-in unix socket via
  `--add-cloudsql-instances` (no public IP, no VPC connector).
- **Database:** one Cloud SQL for PostgreSQL instance, `db-f1-micro` shared-core,
  zonal (no HA), minimal SSD. Three databases: `tallyo_dev`, `tallyo_stg`,
  `tallyo_prd`, each with a dedicated database user/password.
- **Registry:** one Artifact Registry Docker repository in the region. Images are
  built and pushed (manually or by future CI), and each Cloud Run service deploys
  a tagged image.
- **Secrets:** each env's DB password and the `ANTHROPIC_API_KEY` live in Secret
  Manager and are injected into the Cloud Run service as environment variables /
  secret refs. `DATABASE_URL` for each service points at its database via the
  Cloud SQL socket.
- **Service accounts:** a Cloud Run runtime SA per env (least privilege: Cloud SQL
  client, Secret Manager accessor for its own secrets). A Scheduler SA permitted
  to invoke the Cloud Run service (`roles/run.invoker`).
- **Sweep schedule:** one Cloud Scheduler job per environment, hourly, OIDC-authed
  to that env's Cloud Run service `/api/internal/sweep`.

---

## 4. docker-compose (local dev)

- A new multi-stage `Dockerfile`:
  1. Build the SvelteKit SPA (`web/`, `npm ci && npm run build`).
  2. Build the cgo-free Go binary (`CGO_ENABLED=0 go build ./cmd/tallyo`) with the
     SPA embedded.
  3. Final stage: a distroless (or `gcr.io/distroless/static`) image running the
     binary.
- `docker-compose.yml`:
  - `postgres:17` service with a named volume and healthcheck.
  - `app` service built from the Dockerfile, `DATABASE_URL` pointing at the
    compose Postgres, `SWEEP_TICKER=1`, depends-on Postgres healthy.
  - One `docker compose up` yields a working local stack; migrations run on app
    startup.
- The same Dockerfile image is what Artifact Registry hosts and Cloud Run runs.

---

## 5. IaC: OpenTofu modules + Terragrunt

### 5.1 Layout

```
infra/
  modules/                      # reusable OpenTofu modules (provider-agnostic of env)
    project-services/           # enable required GCP APIs
    artifact-registry/          # one Docker repo (project/region scoped)
    cloud-sql/                  # ONE shared instance + per-env database + user
    secrets/                    # Secret Manager secrets + versions
    cloud-run/                  # one Cloud Run service + runtime SA + IAM
    scheduler/                  # Cloud Scheduler job + invoker SA binding
  live/
    terragrunt.hcl              # root: GCS remote state, google provider codegen, common inputs
    _envcommon/                 # shared per-module input fragments (DRY)
    <project>/                  # e.g. tallyo
      project.hcl               # project_id var
      <region>/                 # e.g. australia-southeast1
        region.hcl              # region var
        artifact-registry/      # shared by all envs in this region
        cloud-sql/              # ONE shared instance for all envs
        dev/                    # per-env leaf
          database/  secrets/  cloud-run/  scheduler/
        stg/                    # same shape
        prd/                    # same shape
```

### 5.2 Principles

- Each leaf is a Terragrunt unit with its own `terragrunt.hcl` that points at a
  module in `infra/modules/` and pulls common inputs from `_envcommon/`,
  `region.hcl`, and `project.hcl` via Terragrunt's `read_terragrunt_config` /
  `find_in_parent_folders`.
- Remote state: one GCS bucket, state keyed by path so each leaf has isolated
  state. The root `terragrunt.hcl` generates the `backend` and `google` provider
  blocks so no module hard-codes them.
- The shared `cloud-sql` unit owns the **instance only**. Each env owns a small
  per-env `database/` leaf (creating that env's `google_sql_database` +
  `google_sql_user`) that declares a Terragrunt `dependency` on the shared
  `cloud-sql` unit and consumes its `instance_name` output. (Decision made: the
  per-env-leaf form, not feeding three databases as inputs to the shared module —
  it keeps each env's DB lifecycle in its own state and matches the per-env leaf
  structure.) The constraint holds: one instance, three databases.
- **Deploy now:** apply only `<project>/<region>/{artifact-registry, cloud-sql,
  dev, stg, prd}`. Adding a region = copy `<region>/` to a new region dir; adding
  a project = copy `<project>/`. No module edits required for either.

### 5.3 Bootstrap

- The GCS state bucket and initial project API enablement are a documented
  one-time bootstrap (either a tiny separate tofu root applied with local state,
  or documented `gcloud` commands). The plan specifies which.

---

## 6. Risks and decisions

- **Identity shift:** dropping SQLite removes the zero-dependency single-binary
  self-host story. Accepted by the user.
- **Cold-start latency:** scale-to-zero means the first request after idle
  (including a Scheduler sweep) pays a cold start. Accepted for cost; a modest
  `max-instances` cap bounds instance sprawl.
- **Polling vs realtime:** dropping SSE means cross-user updates appear on the
  next poll/focus (~30s) instead of instantly. Accepted for a single-org app; the
  trade buys a stateless server and a smaller codebase.
- **`db-f1-micro` limits:** shared-core, no HA, limited connections. Fits a
  single-org low-traffic app; revisit if load grows.
- **One shared Cloud SQL instance across dev/stg/prd:** an instance-level outage
  or noisy `dev` affects `prd`. Accepted for cost at this stage; the IaC layout
  lets `prd` move to its own instance later by pointing its `cloud-sql`
  dependency elsewhere.
- **Test DB dependency:** Go tests now require a reachable Postgres. The plan must
  keep `go test ./...` ergonomic (skip-when-unset or testcontainers).

## 7. Acceptance criteria

- `go test ./...` passes against Postgres (including the numbering concurrency
  test); `go vet ./...` and `gofmt -l .` clean; `CGO_ENABLED=0 go build
  ./cmd/tallyo` succeeds; `cd web && npm run check` clean.
- No `modernc.org/sqlite` (incl. the blank import and the `go.mod` require),
  `sqlite3store`, or SQLite pragma/`_txlock` references remain. No `BLOB` /
  `CAST(... AS REAL)` / `date('now')` / `_pragma` strings remain in migrations
  or queries.
- `isRetryable` matches pgconn SQLSTATEs (`23505`/`40001`), and the numbering
  concurrency test passes against Postgres.
- `internal/realtime/` and `internal/events/` are gone; no `realtime.`,
  `events.Notifier`, `EventSource`, or `/api/events` references remain (Go or
  SPA). Services no longer take a notifier. The SPA polls (interval + focus) and
  still refreshes after a mutation.
- `docker compose up` brings up Postgres + app; the app migrates and serves.
- `tofu`/`terragrunt` plan validates for the live leaves; applying provisions
  Artifact Registry, one Cloud SQL instance with three databases, three Cloud Run
  services, three Cloud Scheduler jobs, and the supporting SAs/secrets.
- Adding a new region or project requires only new `live/` leaf directories, no
  module changes.
