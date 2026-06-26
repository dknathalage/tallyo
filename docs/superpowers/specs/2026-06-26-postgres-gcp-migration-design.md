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

This work also **simplifies the app to a single stateless web binary** (§2): the
realtime/SSE subsystem is removed (the SPA polls), **all background processing is
removed** (the overdue sweep — overdue becomes a UI-derived display — and the
recurring-invoice sweep, whose whole feature is dropped), and there is no worker,
task queue, or scheduler. With no in-process connection state and no background
work, Cloud Run scale-to-zero is trivially correct. Net: a meaningful code
deletion.

This changes Tallyo's identity from "single self-hosted binary with embedded
SQLite" to a Postgres-backed web service (still a single binary). That trade-off
was chosen deliberately.

## Goals

- Replace SQLite with Postgres across driver, sqlc, goose, and session store.
- Remove the SSE realtime subsystem; the SPA polls instead.
- Remove all background work: delete the overdue sweep (overdue → UI-derived) and
  remove the recurring-invoice feature entirely. Server becomes stateless with no
  ticker/worker/scheduler.
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
- Any background worker, task queue (Cloud Tasks/Pub/Sub), or scheduler (Cloud
  Scheduler) — explicitly out. There is no background work to run.
- Scheduled/recurring invoice generation — the feature is removed, not deferred.
- Offloading slow user-triggered work (LLM Smarts, large imports, PDF) to async —
  out of scope; they stay synchronous as today. If async is needed later it is a
  separate worker design.

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
  - `date('now')` — **not ported, deleted.** Its only use was
    `SelectOverdueInvoicesForTenant` (`invoices.sql:79`), which is removed entirely
    with the overdue sweep (§2.2). No Postgres date-function rewrite is needed.
  - `recurring_templates.sql` — **deleted** with the recurring feature (§2.3); it
    is not ported.
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
  `catalogue_items.unit_price`, etc.). `REAL` is valid PG but is 4-byte float
  (lossy); `double precision` (8-byte) matches Go `float64` and the current SQLite
  REAL→float64 behavior. All `REAL` → `double precision`. (The
  `recurring_templates` table is deleted, not converted — §2.3.)
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

## 2. Statelessness & simplification

Alongside the Postgres move, this work removes everything that ties the app to an
always-on or single instance, plus one feature the user has decided to drop. End
state: a **single stateless web binary** (`cmd/tallyo`, unchanged) with **no
background work** — so Cloud Run scale-to-zero is trivially correct. There is no
worker, no task queue, no scheduler, no ticker.

### 2.1 Remove the realtime/SSE subsystem → client polling

Backend (net deletion):

- **Delete `internal/realtime/`** (hub, `/api/events` SSE handler, tests) and
  **`internal/events/`** (the `events.Notifier`).
- **Remove broadcast calls from every service.** The service files that construct
  /use a notifier and call `Created/Updated/Deleted`
  (`internal/{catalogue,client,estimate,invoice,payer,taxrate}/service.go`,
  `internal/session/{service.go,service_items.go}`) drop the notifier field,
  constructor param, and `s.events.*` calls. Audit (`audit.WithTx`) is unaffected.
- **`internal/app`:** remove the `Events *realtime.EventsHandler` field and the
  `pr.Get("/events", …)` route in `server.go`; remove hub wiring in `app.go`;
  delete/adjust the app tests that build `realtime.NewHub()`.
- Update the CLAUDE.md convention "broadcasts an SSE event from the service after
  commit" — that behavior is gone.

Frontend (mechanism swap; the client already resyncs by refetching):

- **Delete `web/src/lib/realtime/events.ts`** and the `openEvents()/closeEvents()`
  calls in `web/src/routes/[tenant]/+layout.svelte`.
- Add a ~20-line poll helper (`web/src/lib/realtime/poll.ts`): given a refetch
  callback, refetch on mount, on an interval (~30s), and on
  `visibilitychange`/focus; return a cleanup. (`ponytail: fixed 30s interval +
  focus refetch; tune only if stale or chatty`.)
- Replace the `onEntity(entity, cb)` subscriptions in the three stores
  (`stores/sessions.svelte.ts`, `stores/collection.svelte.ts` — the generic CRUD
  store — `stores/businessProfile.svelte.ts`) with the poll helper bound to the
  same refetch. SvelteKit `load` already refetches on navigation, so polling only
  covers "data changed while sitting on a view."

### 2.2 Remove all background sweeps; overdue becomes UI-derived

There is no background processing of any kind after this change.

- **Overdue is a UI concern, not a persisted/swept status.** "Overdue" =
  `status='sent' AND due_date < today`, which the SPA computes from the data it
  already has (the dashboard already does this at `+page.svelte:29`). The API
  returns the **raw** stored status (`sent`); no backend derivation is added.
- **Delete the overdue flip machinery:** `MarkOverdueForTenant` (invoice service +
  repo), `flipOverdue`, the `SelectOverdueInvoicesForTenant` query, the
  `OverdueInvoice` type, the **read-time sweep in `Handler.List`**
  (`invoice/handler.go:127`), and the sent→overdue audit writes. This also deletes
  the `date('now')` query — removing that Postgres-port item entirely.
- **Delete `internal/app/sweep.go`** (`runSweeper`/`runSweepOnce`) and its
  goroutine launch in `app.go`. No ticker, no `SWEEP_TICKER`, no Cloud Scheduler.
- **Frontend overdue:** the invoice list "overdue" filter chip and the detail
  badge move to client-side computation from `due_date` (status `sent` + past due
  → show as overdue). The follow-up Smart button already shows for `sent`, so it
  still appears for now-overdue invoices.

### 2.3 Remove the recurring feature (full deletion)

The recurring-invoice generator is real scheduled backend work; with no
background processing it cannot run, and the user has chosen to drop the feature
rather than keep any scheduler. Remove it end to end:

- **Backend:** delete the `internal/recurring/` slice (all files); remove the
  `recurring_templates` table from the baseline migration
  (`tenant/00001_tenant.sql`, clean-break so edit baseline in place) and its
  indexes; delete `internal/db/queries/recurring_templates.sql` and regenerate
  `gen/` (drops the `RecurringTemplate` model + all its query methods); remove the
  `internal/app` wiring — the `Recurring` handler field + route in `server.go`,
  `recurring.NewService`/`NewHandler` in `app.go`, the `FeatureRecurring` config
  field, the `"recurring"` features-map entry, and `recForSweep`.
- **Frontend:** delete `web/src/lib/stores/recurring.svelte.ts` and
  `web/src/routes/[tenant]/recurring/`; remove the recurring entry from
  `stores/features.svelte.ts`, the nav link in `+layout.svelte`, and the
  recurring types in `api/types.ts`.
- **Gate:** the `FeatureRecurring` env gate is removed entirely (not just
  defaulted off).

### 2.4 Cloud Run configuration (single service)

- One Cloud Run service per env (`tallyo`, the existing binary). `min-instances=0`
  (true scale-to-zero — correct now that there is no background work and no
  in-process connection state), `concurrency=80`, a modest `max-instances` cap
  (e.g. 3) purely as a cost guardrail. Cloud Run gen2.
- No worker service, no Cloud Tasks queue, no Cloud Scheduler.

---

## 3. GCP architecture (cheap, single project now)

- **Compute:** one Cloud Run service per environment (`dev`, `stg`, `prd`) in one
  project/region — the single `tallyo` binary, scale-to-zero. Each connects to
  Cloud SQL through the built-in unix socket via `--add-cloudsql-instances` (no
  public IP, no VPC connector). No worker, no task queue, no scheduler.
- **Database:** one Cloud SQL for PostgreSQL instance, `db-f1-micro` shared-core,
  zonal (no HA), minimal SSD. Three databases: `tallyo_dev`, `tallyo_stg`,
  `tallyo_prd`, each with a dedicated database user/password.
- **Registry:** one Artifact Registry Docker repository in the region. One image
  (`tallyo`) is built and pushed (manually or by future CI); each Cloud Run
  service deploys its tagged image.
- **Secrets:** each env's DB password and the `ANTHROPIC_API_KEY` live in Secret
  Manager, injected into the service. `DATABASE_URL` points at the env's database
  via the Cloud SQL socket.
- **Service accounts:** one Cloud Run runtime SA per env (least privilege: Cloud
  SQL client, Secret Manager accessor for its own secrets).
- **Ingress:** the service is the only (public-facing) component per env.

---

## 4. docker-compose (local dev)

- A multi-stage `Dockerfile`:
  1. Build the SvelteKit SPA (`web/`, `npm ci && npm run build`).
  2. Build the cgo-free binary: `CGO_ENABLED=0 go build ./cmd/tallyo` (SPA
     embedded).
  3. Final distroless (`gcr.io/distroless/static`) stage running the binary.
- `docker-compose.yml`:
  - `postgres:17` service with a named volume and healthcheck.
  - `app` service (`tallyo` image), `DATABASE_URL` at the compose Postgres,
    depends-on Postgres healthy.
  - One `docker compose up` yields a working local stack (Postgres + app);
    migrations run on app startup.
- The same image is what Artifact Registry hosts and Cloud Run runs.

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
          database/  secrets/  cloud-run/
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
- The per-env `cloud-run` leaf declares `dependency` on that env's `database`,
  `secrets`, and the shared `cloud-sql` unit (for the instance connection name).
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
- **Cold-start latency:** scale-to-zero means the first user request after idle
  pays a cold start. Accepted for cost; a modest `max-instances` cap bounds
  instance sprawl.
- **Polling vs realtime:** dropping SSE means cross-user updates appear on the
  next poll/focus (~30s) instead of instantly. Accepted for a single-org app; the
  trade buys a stateless server and a smaller codebase.
- **No background processing:** with both sweeps removed, nothing runs unless a
  request drives it. Overdue is computed in the UI; recurring invoices are no
  longer auto-generated (feature removed). Accepted by the user — if scheduled
  generation is ever wanted again, it returns as a separate worker/scheduler
  design, not retrofitted here.
- **Recurring feature removed:** existing recurring templates and the table are
  deleted (clean-break schema, no data to preserve). Irreversible by design.
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
  SPA). Services no longer take a notifier. The SPA polls (interval + focus).
- **No background work:** `internal/app/sweep.go` is gone; no ticker, no
  `MarkOverdueForTenant`/`flipOverdue`/`SelectOverdueInvoicesForTenant`, no
  `SWEEP_TICKER`. The invoice API returns the raw stored status; the SPA shows
  "overdue" computed from `due_date`.
- **Recurring feature removed:** no `internal/recurring/`, no `recurring_templates`
  table/queries/`RecurringTemplate` model, no `FeatureRecurring` gate, no
  `web/.../recurring/` routes — `grep -ri recurring` is clean across Go and SPA.
- `docker compose up` brings up Postgres + app; the app migrates and serves.
- `tofu`/`terragrunt` plan validates for the live leaves; applying provisions
  Artifact Registry, one Cloud SQL instance with three databases, **three** Cloud
  Run services (one per env), and the supporting SAs/secrets. No Cloud Tasks, no
  Cloud Scheduler.
- Adding a new region or project requires only new `live/` leaf directories, no
  module changes.
