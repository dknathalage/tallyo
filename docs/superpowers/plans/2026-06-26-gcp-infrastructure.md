# Tallyo GCP Infrastructure Plan (Plan 3 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to execute this plan. Work top-to-bottom, one task at a time, and tick every checkbox (`- [ ]`) as you complete its step. Do not skip per-task verification. Do NOT run `terragrunt apply` as part of automated execution — `apply` (and `plan` against real cloud) is gated behind an explicit human step (see Apply Gate): it requires real GCP credentials, an existing project, and active billing.

**Goal:** Stand up Tallyo's GCP infra as reusable OpenTofu modules orchestrated by Terragrunt — single project / single region now, laid out so adding a region or project is a directory copy with zero module edits. Deployed surface: one Artifact Registry Docker repo, one shared Cloud SQL Postgres instance with three databases, three scale-to-zero Cloud Run services (`dev`/`stg`/`prd`) running the single `tallyo` image, per-env Secret Manager secrets, per-env least-privilege runtime service accounts.

**Architecture:** Cheapest-viable, single-project-now, multi-region/multi-project-capable. No account layer. One Cloud Run service per env (gen2, `min=0`, modest `max`, concurrency 80) reaching Cloud SQL over the built-in unix socket via `--add-cloudsql-instances` (no public IP, no VPC connector). `DATABASE_URL = postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT:REGION:INSTANCE`. One shared `db-f1-micro` zonal Cloud SQL instance; three databases (`tallyo_dev`/`tallyo_stg`/`tallyo_prd`), each its own user+password. One Artifact Registry Docker repo. Secret Manager holds each env's DB password + `ANTHROPIC_API_KEY`. No worker, no Cloud Tasks, no Cloud Scheduler.

**Tech Stack:** OpenTofu (`tofu`), Terragrunt, GCP (Cloud Run, Cloud SQL for PostgreSQL, Artifact Registry, Secret Manager, IAM), `hashicorp/google` `~> 6.0`, GCS remote state.

**Spec:** docs/superpowers/specs/2026-06-26-postgres-gcp-migration-design.md (§3, §5, §1.1, §2.4)

**Dependency:** Depends only on a `tallyo` image existing in Artifact Registry (Plan 2). Independent of app code (Plan 1). The `cloud-run` module takes the image ref as input; until a real image is pushed, `cloud-run` `apply` is the last step.

---

## Prerequisites (before any `plan`/`apply`)

`tofu fmt -check`, `tofu validate`, `terragrunt hclfmt --terragrunt-check`, and `terragrunt run-all validate` run with **no cloud credentials** — the automated gate in every task. But `terragrunt plan`/`apply` require real GCP creds, an existing project with **billing**, and the bootstrap state bucket:
```bash
gcloud auth application-default login
gcloud config set project <PROJECT_ID>
```
Tools required: `tofu` (>= 1.8), `terragrunt` (>= 0.67), `gcloud`. **Placeholders:** project id `tallyo`, region `australia-southeast1` — both are variables; nothing in `modules/` hard-codes them.

---

## File-structure map

```
infra/
  bootstrap/                       # one-time, LOCAL state — GCS state bucket + core APIs
    main.tf variables.tf outputs.tf terraform.tfvars.example README.md
  modules/                         # reusable OpenTofu modules (env-agnostic)
    project-services/  artifact-registry/  cloud-sql/  database/  secrets/  cloud-run/
      (each: main.tf variables.tf outputs.tf)
  live/
    terragrunt.hcl                 # ROOT: GCS backend + google provider codegen, common inputs
    _envcommon/                    # shared per-module input fragments (DRY)
      artifact-registry.hcl  cloud-sql.hcl  database.hcl  secrets.hcl  cloud-run.hcl
    tallyo/  project.hcl           # <project>
      australia-southeast1/  region.hcl   # <region>
        artifact-registry/  cloud-sql/    # region-shared
        dev/  database/ secrets/ cloud-run/   # per-env leaves
        stg/  database/ secrets/ cloud-run/
        prd/  database/ secrets/ cloud-run/
  .gitignore
  README.md
```

**Module split note (vs spec §5.1):** spec's `cloud-sql` bullet says "instance + per-env db + user", but §5.2 decided each env owns its own `database/` leaf with its own state. To honor that cleanly this plan splits into `cloud-sql/` (instance only) + `database/` (per-env db+user). Six modules total. No behavioral difference from the spec.

---

## Task 1 — Bootstrap root (GCS state bucket + core APIs, local state)

**Decision (spec §5.3, recommended option):** a tiny `infra/bootstrap/` tofu root with **local state** — it creates the GCS bucket backing every other unit's remote state (chicken-and-egg) and enables core APIs. Idempotent, reviewable, re-runnable; preferred over loose `gcloud` commands.

**Files:** `infra/bootstrap/{main.tf,variables.tf,outputs.tf,terraform.tfvars.example,README.md}`, `infra/.gitignore`.

- [ ] **Step 1:** Create `infra/.gitignore`:
  ```gitignore
  **/.terraform/*
  **/.terragrunt-cache/*
  *.tfstate
  *.tfstate.*
  crash.log
  crash.*.log
  *.tfvars
  !*.tfvars.example
  override.tf
  override.tf.json
  *_override.tf
  *_override.tf.json
  ```
  Note: do NOT ignore `.terraform.lock.hcl` — commit provider lock files for reproducible provider versions across machines/CI.
- [ ] **Step 2:** `infra/bootstrap/variables.tf`:
  ```hcl
  variable "project_id" { type = string }
  variable "region" { type = string, default = "australia-southeast1" }
  variable "state_bucket_name" { description = "Globally-unique GCS bucket for tofu state.", type = string }
  variable "core_apis" {
    type = list(string)
    default = ["serviceusage.googleapis.com", "cloudresourcemanager.googleapis.com", "iam.googleapis.com", "storage.googleapis.com"]
  }
  ```
  (Note: split the `region` defaults onto separate lines — HCL doesn't allow `type` and `default` on one line with a comma; write `type = string` then `default = "..."` on its own line.)
- [ ] **Step 3:** `infra/bootstrap/main.tf`:
  ```hcl
  terraform {
    required_version = ">= 1.8.0"
    required_providers {
      google = { source = "hashicorp/google", version = "~> 6.0" }
    }
  }
  provider "google" {
    project = var.project_id
    region  = var.region
  }
  resource "google_project_service" "core" {
    for_each                   = toset(var.core_apis)
    project                    = var.project_id
    service                    = each.value
    disable_dependent_services = false
    disable_on_destroy         = false
  }
  resource "google_storage_bucket" "state" {
    name                        = var.state_bucket_name
    project                     = var.project_id
    location                    = var.region
    force_destroy               = false
    uniform_bucket_level_access = true
    versioning { enabled = true }
    lifecycle { prevent_destroy = true }
    depends_on = [google_project_service.core]
  }
  ```
- [ ] **Step 4:** `infra/bootstrap/outputs.tf` (`state_bucket_name`, `enabled_core_apis`); `terraform.tfvars.example` (`project_id`/`region`/`state_bucket_name`); `README.md` documenting `gcloud auth application-default login` + billing, `cp terraform.tfvars.example terraform.tfvars`, `tofu init && tofu apply`, and that this is a one-time human step.
- [ ] **Step 5:** Verify (no creds): `cd infra/bootstrap && tofu fmt -check && tofu init -backend=false && tofu validate` → fmt clean; `Success! The configuration is valid.`
- [ ] **Step 6 (human apply gate — not automated):** with creds+billing, `cp ... && tofu init && tofu apply` → bucket + APIs created; record bucket name for `live/terragrunt.hcl`.
- [ ] **Step 7:** Commit: `feat(infra): bootstrap tofu root for GCS state bucket + core APIs`

---

## Task 2 — Module `project-services`

**Files:** `infra/modules/project-services/{main.tf,variables.tf,outputs.tf}`.

- [ ] **Step 1:** `variables.tf`: `project_id` (string); `services` (list(string)) default `["run.googleapis.com","sqladmin.googleapis.com","artifactregistry.googleapis.com","secretmanager.googleapis.com","iam.googleapis.com","compute.googleapis.com"]`.
- [ ] **Step 2:** `main.tf`: `terraform{}` block (google ~> 6.0) + `google_project_service "this"` `for_each = toset(var.services)`, `disable_on_destroy = false`.
- [ ] **Step 3:** `outputs.tf`: `enabled_services = sort([for s in google_project_service.this : s.service])`.
- [ ] **Step 4:** `cd infra/modules/project-services && tofu fmt -check && tofu init -backend=false && tofu validate` → clean + valid.
- [ ] **Step 5:** Commit: `feat(infra): add project-services module to enable GCP APIs`

---

## Task 3 — Module `artifact-registry`

**Files:** `infra/modules/artifact-registry/{main.tf,variables.tf,outputs.tf}`.

- [ ] **Step 1:** `variables.tf`: `project_id`, `region`, `repository_id` (default `"tallyo"`), `description`.
- [ ] **Step 2:** `main.tf`: `google_artifact_registry_repository "this"` `format = "DOCKER"`, `location = var.region`.
- [ ] **Step 3:** `outputs.tf`: `repository_id`, `repository_name`, and `registry_url = "${var.region}-docker.pkg.dev/${var.project_id}/${...repository_id}"`.
- [ ] **Step 4:** `tofu fmt -check && tofu init -backend=false && tofu validate` → clean + valid.
- [ ] **Step 5:** Commit: `feat(infra): add artifact-registry module (one Docker repo)`

---

## Task 4 — Module `cloud-sql` (shared instance only)

**Files:** `infra/modules/cloud-sql/{main.tf,variables.tf,outputs.tf}`.

- [ ] **Step 1:** `variables.tf`: `project_id`, `region`, `instance_name` (default `"tallyo-pg"`), `database_version` (default `"POSTGRES_17"`), `tier` (default `"db-f1-micro"`), `disk_size_gb` (default 10), `deletion_protection` (default true).
- [ ] **Step 2:** `main.tf`: `google_sql_database_instance "this"` with `settings { tier = var.tier; availability_type = "ZONAL"; disk_type = "PD_SSD"; disk_size = var.disk_size_gb; disk_autoresize = true; ip_configuration { ipv4_enabled = false }; backup_configuration { enabled = true } }`, `deletion_protection = var.deletion_protection`. (No public IP, no VPC — access only via the Cloud SQL socket.)
- [ ] **Step 3:** `outputs.tf`: `instance_name`, `connection_name` (PROJECT:REGION:INSTANCE — used by `--add-cloudsql-instances` + the DSN `host=` param).
- [ ] **Step 4:** `tofu fmt -check && tofu init -backend=false && tofu validate` → clean + valid.
- [ ] **Step 5:** Commit: `feat(infra): add cloud-sql module (one shared zonal instance)`

---

## Task 5 — Module `database` (per-env db + user)

**Files:** `infra/modules/database/{main.tf,variables.tf,outputs.tf}`.

- [ ] **Step 1:** `variables.tf`: `project_id`, `instance_name`, `database_name`, `user_name`, `user_password` (sensitive).
- [ ] **Step 2:** `main.tf`: `google_sql_database "this"` (instance + name) + `google_sql_user "this"` (instance + name + password).
- [ ] **Step 3:** `outputs.tf`: `database_name`, `user_name`.
- [ ] **Step 4:** `tofu fmt -check && tofu init -backend=false && tofu validate` → clean + valid.
- [ ] **Step 5:** Commit: `feat(infra): add database module (per-env db + user)`

---

## Task 6 — Module `secrets` (Secret Manager per-env)

Generates the DB password (so it has a single source of truth per env), creates the DB-password + ANTHROPIC_API_KEY secret containers with regional replication, adds the DB-password version always and the Anthropic version only when a value is supplied.

**Files:** `infra/modules/secrets/{main.tf,variables.tf,outputs.tf}`.

- [ ] **Step 1:** `variables.tf`: `project_id`, `env`, `region`, `anthropic_api_key` (default `""`, sensitive).
- [ ] **Step 2:** `main.tf`: providers `google ~> 6.0` + `random ~> 3.6`; `random_password "db"` (length 32, `special = false` to avoid DSN-encoding hazards); `google_secret_manager_secret "db_password"` (`secret_id = "tallyo-${var.env}-db-password"`, `replication { user_managed { replicas { location = var.region } } }`) + always-on version `= random_password.db.result`; `google_secret_manager_secret "anthropic"` (`tallyo-${var.env}-anthropic-api-key`) + `google_secret_manager_secret_version "anthropic"` `count = var.anthropic_api_key == "" ? 0 : 1`.
- [ ] **Step 3:** `outputs.tf`: `db_password` (sensitive), `db_password_secret_id`, `anthropic_secret_id`.
- [ ] **Step 4:** `tofu fmt -check && tofu init -backend=false && tofu validate` → clean + valid.
- [ ] **Step 5:** Commit: `feat(infra): add secrets module (per-env DB password + ANTHROPIC_API_KEY)`

---

## Task 7 — Module `cloud-run` (service + runtime SA + IAM)

One scale-to-zero gen2 service per env: least-privilege runtime SA (`roles/cloudsql.client` + `secretmanager.secretAccessor` scoped to the two env secrets), `DATABASE_URL` from the socket form, `ANTHROPIC_API_KEY` from Secret Manager `latest`, Cloud SQL volume attached, optional public invoker.

**Files:** `infra/modules/cloud-run/{main.tf,variables.tf,outputs.tf}`.

- [ ] **Step 1:** `variables.tf`: `project_id`, `region`, `env`, `service_name`, `image`; `cloudsql_connection_name`, `db_name`, `db_user`, `db_password` (sensitive); `anthropic_secret_id`, `db_password_secret_id`; scaling `min_instances` (0), `max_instances` (3), `concurrency` (80), `cpu` ("1"), `memory` ("512Mi"); `allow_public` (default true).
- [ ] **Step 2:** `main.tf`:
  ```hcl
  terraform {
    required_version = ">= 1.8.0"
    required_providers { google = { source = "hashicorp/google", version = "~> 6.0" } }
  }
  locals {
    database_url = format("postgres://%s:%s@/%s?host=/cloudsql/%s",
      var.db_user, var.db_password, var.db_name, var.cloudsql_connection_name)
  }
  resource "google_service_account" "runtime" {
    project      = var.project_id
    account_id   = "tallyo-${var.env}-run"
    display_name = "Tallyo ${var.env} Cloud Run runtime"
  }
  resource "google_project_iam_member" "cloudsql_client" {
    project = var.project_id
    role    = "roles/cloudsql.client"
    member  = "serviceAccount:${google_service_account.runtime.email}"
  }
  resource "google_secret_manager_secret_iam_member" "anthropic" {
    project   = var.project_id
    secret_id = var.anthropic_secret_id
    role      = "roles/secretmanager.secretAccessor"
    member    = "serviceAccount:${google_service_account.runtime.email}"
  }
  resource "google_secret_manager_secret_iam_member" "db_password" {
    project   = var.project_id
    secret_id = var.db_password_secret_id
    role      = "roles/secretmanager.secretAccessor"
    member    = "serviceAccount:${google_service_account.runtime.email}"
  }
  resource "google_cloud_run_v2_service" "this" {
    project             = var.project_id
    name                = var.service_name
    location            = var.region
    deletion_protection = false
    template {
      service_account = google_service_account.runtime.email
      scaling {
        min_instance_count = var.min_instances
        max_instance_count = var.max_instances
      }
      max_instance_request_concurrency = var.concurrency
      execution_environment            = "EXECUTION_ENVIRONMENT_GEN2"
      volumes {
        name = "cloudsql"
        cloud_sql_instance { instances = [var.cloudsql_connection_name] }
      }
      containers {
        image = var.image
        resources { limits = { cpu = var.cpu, memory = var.memory } }
        env {
          name  = "DATABASE_URL"
          value = local.database_url
        }
        env {
          name = "ANTHROPIC_API_KEY"
          value_source { secret_key_ref { secret = var.anthropic_secret_id, version = "latest" } }
        }
        volume_mounts { name = "cloudsql", mount_path = "/cloudsql" }
      }
    }
    depends_on = [
      google_secret_manager_secret_iam_member.anthropic,
      google_secret_manager_secret_iam_member.db_password,
      google_project_iam_member.cloudsql_client,
    ]
  }
  resource "google_cloud_run_v2_service_iam_member" "public" {
    count    = var.allow_public ? 1 : 0
    project  = var.project_id
    location = var.region
    name     = google_cloud_run_v2_service.this.name
    role     = "roles/run.invoker"
    member   = "allUsers"
  }
  ```
  (Note: where a `{ }` block above is written inline with commas — e.g. `resources { limits = {...} }`, `secret_key_ref {...}`, `volume_mounts {...}` — expand to proper multi-line HCL; commas inside `{ }` argument maps are fine, but block bodies use newlines.)
- [ ] **Step 3:** `outputs.tf`: `service_name`, `url = google_cloud_run_v2_service.this.uri`, `runtime_service_account`.
- [ ] **Step 4:** `tofu fmt -check && tofu init -backend=false && tofu validate` → clean + valid.
- [ ] **Step 5:** Commit: `feat(infra): add cloud-run module (scale-to-zero svc + runtime SA + IAM)`

---

## Task 8 — Live root + project/region config

**Files:** `infra/live/terragrunt.hcl`, `infra/live/tallyo/project.hcl`, `infra/live/tallyo/australia-southeast1/region.hcl`.

- [ ] **Step 1:** `live/terragrunt.hcl`:
  ```hcl
  locals {
    project      = read_terragrunt_config(find_in_parent_folders("project.hcl"))
    region       = read_terragrunt_config(find_in_parent_folders("region.hcl"))
    project_id   = local.project.locals.project_id
    region_id    = local.region.locals.region
    state_bucket = "tallyo-tofu-state" # MUST equal the bucket created in Task 1 (bootstrap state_bucket_name). GCS bucket names are globally unique — if "tallyo-tofu-state" is taken, pick another and use the SAME value in both infra/bootstrap/terraform.tfvars AND here.
  }
  remote_state {
    backend = "gcs"
    generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }
    config = {
      project  = local.project_id
      location = local.region_id
      bucket   = local.state_bucket
      prefix   = "${path_relative_to_include()}/tofu.tfstate"
    }
  }
  generate "provider" {
    path      = "provider.tf"
    if_exists = "overwrite_terragrunt"
    contents  = <<-EOF
      terraform {
        required_version = ">= 1.8.0"
        required_providers {
          google = { source = "hashicorp/google", version = "~> 6.0" }
          random = { source = "hashicorp/random", version = "~> 3.6" }
        }
      }
      provider "google" {
        project = "${local.project_id}"
        region  = "${local.region_id}"
      }
    EOF
  }
  inputs = {
    project_id = local.project_id
    region     = local.region_id
  }
  ```
- [ ] **Step 2:** `project.hcl`: `locals { project_id = "tallyo" }` (placeholder). `region.hcl`: `locals { region = "australia-southeast1" }`.
- [ ] **Step 3:** `cd infra/live && terragrunt hclfmt --terragrunt-check` → no diff (exit 0).
- [ ] **Step 4:** Commit: `feat(infra): add live terragrunt root + project/region config`

---

## Task 9 — `_envcommon` shared fragments (DRY)

Each fragment sets `terraform.source` (module) + common inputs + Terragrunt `dependency` blocks with `mock_outputs` (so `validate`/`plan`/`init` run without a real apply).

**Files:** `infra/live/_envcommon/{artifact-registry,cloud-sql,database,secrets,cloud-run}.hcl`.

- [ ] **Step 1:** `artifact-registry.hcl`: `terraform { source = "${get_repo_root()}/infra/modules//artifact-registry" }`.
- [ ] **Step 2:** `cloud-sql.hcl`: source `//cloud-sql` + `inputs = { instance_name = "tallyo-pg", tier = "db-f1-micro" }`.
- [ ] **Step 3:** `database.hcl`: source `//database`; `dependency "cloud_sql"` (`config_path = "${dirname(dirname(get_terragrunt_dir()))}/cloud-sql"`, `mock_outputs = { instance_name = "tallyo-pg-mock", connection_name = "mock:mock:mock" }`, `mock_outputs_allowed_terraform_commands = ["validate","plan","init"]`); `inputs = { instance_name = dependency.cloud_sql.outputs.instance_name }`.
- [ ] **Step 4:** `secrets.hcl`: source `//secrets` (anthropic empty by default).
- [ ] **Step 5:** `cloud-run.hcl`: source `//cloud-run`; locals compute `image = "${region_id}-docker.pkg.dev/${project_id}/tallyo/tallyo:latest"` (read project/region hcl). Three `dependency` blocks, each with `mock_outputs` + `mock_outputs_allowed_terraform_commands = ["validate","plan","init"]`:
  - `cloud_sql` — **region-level**, `config_path = "${dirname(dirname(get_terragrunt_dir()))}/cloud-sql"` (from `.../<env>/cloud-run` up to `.../<region>/cloud-sql`); `mock_outputs = { instance_name = "tallyo-pg-mock", connection_name = "mock:mock:mock" }`.
  - `database` — **env-level**, `config_path = "${dirname(get_terragrunt_dir())}/database"`; `mock_outputs = { database_name = "tallyo_mock", user_name = "tallyo_mock" }`.
  - `secrets` — **env-level**, `config_path = "${dirname(get_terragrunt_dir())}/secrets"`; `mock_outputs = { db_password = "mock-pw", db_password_secret_id = "tallyo-mock-db-password", anthropic_secret_id = "tallyo-mock-anthropic" }` (must include ALL THREE attributes the inputs wire, or `run-all validate` errors on a missing mock).
  `inputs` wire `image`, `cloudsql_connection_name`, `db_name`, `db_user`, `db_password`, `anthropic_secret_id`, `db_password_secret_id` from those dependency outputs.
- [ ] **Step 6:** `cd infra/live && terragrunt hclfmt --terragrunt-check` → no diff.
- [ ] **Step 7:** Commit: `feat(infra): add _envcommon terragrunt fragments (DRY module wiring)`

---

## Task 10 — Live leaves (region-shared + 3 env leaves)

**Files (region-shared):** `…/australia-southeast1/artifact-registry/terragrunt.hcl`, `…/cloud-sql/terragrunt.hcl`.
**Files (per env, repeat dev/stg/prd):** `…/<env>/{database,secrets,cloud-run}/terragrunt.hcl`.

- [ ] **Step 1:** Region-shared leaves — each just `include "root" { path = find_in_parent_folders() }` + `include "envcommon" { path = "${get_repo_root()}/infra/live/_envcommon/<module>.hcl", expose = true }`.
- [ ] **Step 2:** `dev/database/terragrunt.hcl` — include root + `_envcommon/database.hcl`; a `dependency "secrets"` (`config_path = "${dirname(get_terragrunt_dir())}/secrets"`, mock `db_password`); `inputs = { database_name = "tallyo_dev", user_name = "tallyo_dev", user_password = dependency.secrets.outputs.db_password }`.
- [ ] **Step 3:** `dev/secrets/terragrunt.hcl` — include root + `_envcommon/secrets.hcl`; `inputs = { env = "dev" }`.
- [ ] **Step 4:** `dev/cloud-run/terragrunt.hcl` — include root + `_envcommon/cloud-run.hcl`; `inputs = { env = "dev", service_name = "tallyo-dev" }`.
- [ ] **Step 5:** Repeat Steps 2-4 for `stg` (`tallyo_stg`/`tallyo-stg`) and `prd` (`tallyo_prd`/`tallyo-prd`). Result: 11 leaf files (2 region-shared + 3 envs × 3).
- [ ] **Step 6:** `cd infra/live && terragrunt hclfmt --terragrunt-check && find tallyo -name terragrunt.hcl | sort` → hclfmt clean; exactly the 11 expected leaves.
- [ ] **Step 7:** Commit: `feat(infra): add live leaves (registry, cloud-sql, dev/stg/prd units)`

---

## Task 11 — Validate the whole stack + infra README

**Files:** `infra/README.md`.

- [ ] **Step 1:** `cd infra && tofu fmt -check -recursive modules bootstrap` → no output (run `tofu fmt -recursive ...` + recommit if not).
- [ ] **Step 2:** `cd infra/live && terragrunt hclfmt --terragrunt-check` → exit 0, no diff.
- [ ] **Step 3:** `cd infra/live/tallyo/australia-southeast1 && terragrunt run-all validate --terragrunt-non-interactive` → each unit `Success! The configuration is valid.` (DAG: cloud-sql + secrets → database → cloud-run, using `mock_outputs` where unapplied). If it complains about un-applied deps, confirm each `dependency` lists `validate`/`plan`/`init` in `mock_outputs_allowed_terraform_commands`.
- [ ] **Step 4:** Write `infra/README.md`: prereqs; bootstrap-once; parameterize `project.hcl`/`region.hcl`; set secrets out-of-band (`gcloud secrets versions add tallyo-<env>-anthropic-api-key --data-file=-`); plan/apply order (region-shared → per-env secrets→database→cloud-run, or `terragrunt run-all apply`); image dependency (push `tallyo:latest` before `cloud-run` apply); extending (copy region/project dir, no module edits); apply-gate cost warning.
- [ ] **Step 5:** `cd infra && tofu fmt -check -recursive modules bootstrap && cd live && terragrunt hclfmt --terragrunt-check` → both clean.
- [ ] **Step 6:** Commit: `docs(infra): add infra README (bootstrap, plan/apply runbook, extension)`

---

## Apply Gate (explicit human step — outside automated execution)

Do NOT run during plan execution (needs creds + project + billing; incurs cost):
1. `gcloud auth application-default login` + `gcloud config set project <PROJECT_ID>`.
2. Bootstrap once (Task 1 apply) → state bucket; set it in `live/terragrunt.hcl`.
3. From `infra/live/tallyo/australia-southeast1`: `terragrunt run-all plan`, review.
4. Apply in DAG order (or `terragrunt run-all apply`): `artifact-registry`, `cloud-sql`, then per env `secrets`→`database`→`cloud-run`.
5. Ensure `tallyo:latest` exists in Artifact Registry (Plan 2) before any `cloud-run` apply.
6. Add `ANTHROPIC_API_KEY` secret version per env via `gcloud secrets versions add`.

---

## Plan-level acceptance

- `infra/` exists with `bootstrap/`, six `modules/` (`project-services`, `artifact-registry`, `cloud-sql`, `database`, `secrets`, `cloud-run`), and the `live/` tree (root + `_envcommon` + `tallyo/australia-southeast1/` with `artifact-registry`, `cloud-sql`, and dev/stg/prd each holding `database`/`secrets`/`cloud-run`).
- `tofu fmt -check -recursive infra/modules infra/bootstrap` clean; `tofu validate` passes for every module (`-backend=false`).
- `terragrunt hclfmt --terragrunt-check` clean across `infra/live`; `terragrunt run-all validate` passes for the whole region tree using dependency mocks (no creds).
- No module hard-codes project id or region — both flow from `project.hcl`/`region.hcl` via root `inputs` + generated provider.
- Deployed surface matches spec: ONE Artifact Registry repo, ONE shared `db-f1-micro` zonal Cloud SQL instance with THREE databases each with own user, THREE scale-to-zero gen2 Cloud Run services, per-env runtime SAs (Cloud SQL client + Secret accessor scoped to the env's two secrets), per-env secrets (DB password + ANTHROPIC_API_KEY). NO worker/Cloud Tasks/Cloud Scheduler, NO public IP/VPC, NO account layer.
- `DATABASE_URL` = `postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT:REGION:INSTANCE`; Cloud SQL attached via the `cloud_sql_instance` volume.
- Adding a region/project = copy a `live/` dir + edit its `region.hcl`/`project.hcl`; zero `infra/modules/` changes.
- All commits Conventional, scope `infra`; no `cmd/`/`internal/`/`web/` changes.
