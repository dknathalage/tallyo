terraform {
  required_version = ">= 1.8.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
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
      cloud_sql_instance {
        instances = [var.cloudsql_connection_name]
      }
    }

    containers {
      image = var.image

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }

      env {
        name  = "DATABASE_URL"
        value = local.database_url
      }

      env {
        name = "ANTHROPIC_API_KEY"
        value_source {
          secret_key_ref {
            secret  = var.anthropic_secret_id
            version = "latest"
          }
        }
      }

      volume_mounts {
        name       = "cloudsql"
        mount_path = "/cloudsql"
      }
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
