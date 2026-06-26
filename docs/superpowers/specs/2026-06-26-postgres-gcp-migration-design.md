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

This changes Tallyo's identity from "single self-hosted binary with embedded
SQLite" to a Postgres-backed web service. That trade-off was chosen
deliberately.

## Goals

- Replace SQLite with Postgres across driver, sqlc, goose, and session store.
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
- `timestamptz` schema migration (timestamps stay `TEXT` for now).
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

### 1.2 The `_txlock=immediate` race

SQLite used `_txlock=immediate` to make the numbering slice's read-then-insert
(MAX read + INSERT) take the write lock at `BEGIN`, avoiding
`SQLITE_BUSY_SNAPSHOT`. In Postgres this race is handled by either:

- a `SELECT ... FOR UPDATE` on the relevant counter/row inside the transaction, or
- relying on the existing unique constraint + retry already present in
  `internal/numbering`.

The numbering code's existing retry loop is preserved; the concurrency guard is
re-expressed in Postgres terms (no DSN-level lock mode exists). The exact form is
an implementation detail for the plan, but the behavioral contract — no duplicate
document numbers under concurrency — is unchanged and must be covered by a test.

### 1.3 sqlc

- `sqlc.yaml`: `engine: "sqlite"` → `engine: "postgresql"`.
- Every query in `internal/db/queries/*.sql` uses `?` positional placeholders
  (SQLite style). These are mechanically rewritten to Postgres `$1, $2, …`
  numbered placeholders. `RETURNING *` and `ON CONFLICT (...) DO UPDATE` are
  already valid Postgres and stay.
- The `schema:` list in `sqlc.yaml` continues to point at the migration files
  (control dir + the explicitly-listed tenant files), now interpreted as
  Postgres DDL.
- Regenerate `internal/db/gen/` with `sqlc generate`. The generated Go API
  (method names, params, structs) should remain materially the same; downstream
  slice code is unaffected except where generated types shift.

### 1.4 goose migrations

- `internal/db/migrate.go`: dialect `sqlite3` → `postgres`.
- The two-sequence model (control + tenant, distinct version tables) is kept.
  Goose's Postgres dialect supports custom version-table names.
- Migration DDL is almost entirely `TEXT` columns with `TEXT` PKs (uuidv7) and
  same-file FKs — these port to Postgres unchanged. Review each migration for any
  SQLite-only constructs (none expected beyond pragmas, which don't appear in
  migrations). `IF NOT EXISTS` on the tenant `audit_log` is valid in Postgres.
- Migrations still run on app startup (`appdb.Migrate(db)`), now against the
  environment's Postgres database. One database per environment means each env's
  startup migrates its own database independently.

### 1.5 Session store

- Replace `alexedwards/scs/sqlite3store` with
  `alexedwards/scs/postgresstore`. The scs sessions table is created by the
  postgresstore (or an added migration, matching how it's done today). Session
  semantics are unchanged.

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

## 2. Scale-to-zero reconciliation

Cloud Run scale-to-zero conflicts with two in-process mechanisms: the hourly
overdue+recurring sweep ticker (`internal/app/sweep.go`) and the in-process SSE
hub (`internal/realtime`). Resolution:

### 2.1 Sweep

- Extract the per-tenant sweep into a handler behind an authenticated internal
  endpoint: `POST /api/internal/sweep`.
- **Cloud Scheduler** runs an hourly cron job that calls this endpoint with an
  **OIDC token** (Scheduler → Cloud Run service account). The call wakes the
  scaled-to-zero instance and runs the sweep.
- The in-process hourly ticker is retained **only for local/compose** use, gated
  by an env flag (e.g. `SWEEP_TICKER=1`, off by default). In the cloud, Scheduler
  is the sole driver; locally, the ticker is convenient.
- Endpoint auth: verify the Cloud Run-provided OIDC identity (the request is
  already authenticated by Cloud Run's IAM when the invoker is the Scheduler
  service account). A defense-in-depth shared-secret header is acceptable but
  optional.

### 2.2 SSE

- The browser `EventSource` API auto-reconnects after a connection drop, which is
  what a cold start looks like to the client. Confirm the SPA relies on the
  default reconnect behavior (no code that treats a dropped stream as fatal). No
  server change beyond continuing to accept reconnections.
- Acceptable degradation: a few seconds of missed live updates during cold start;
  the SPA refetches on reconnect.

### 2.3 Cloud Run instance configuration

- `min-instances=0` (true scale-to-zero — chosen for cost).
- `max-instances=1` so the SSE hub and any ticker stay coherent on a single
  instance (single-org app; one instance is sufficient).
- `concurrency=80` (one instance serves all concurrent users).
- Cloud Run gen2 execution environment.

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
          cloud-run/  secrets/  scheduler/   (+ a database/user in the shared instance)
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
- The shared `cloud-sql` unit owns the instance; each env's database + user is
  either a small per-env unit that depends on the shared instance (Terragrunt
  `dependency`) or inputs to the shared module. The plan picks the cleaner of the
  two; the constraint is one instance, three databases.
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
  (including a Scheduler sweep) pays a cold start. Accepted for cost; `max=1`
  bounds instance sprawl.
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
- No `modernc.org/sqlite` / `sqlite3store` references remain.
- `docker compose up` brings up Postgres + app; the app migrates and serves.
- `tofu`/`terragrunt` plan validates for the live leaves; applying provisions
  Artifact Registry, one Cloud SQL instance with three databases, three Cloud Run
  services, three Cloud Scheduler jobs, and the supporting SAs/secrets.
- Adding a new region or project requires only new `live/` leaf directories, no
  module changes.
