terraform {
  required_version = ">= 1.8.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "google_secret_manager_secret" "db_password" {
  project   = var.project_id
  secret_id = "tallyo-${var.env}-db-password"

  replication {
    user_managed {
      replicas {
        location = var.region
      }
    }
  }
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = random_password.db.result
}

resource "google_secret_manager_secret" "anthropic" {
  project   = var.project_id
  secret_id = "tallyo-${var.env}-anthropic-api-key"

  replication {
    user_managed {
      replicas {
        location = var.region
      }
    }
  }
}

resource "google_secret_manager_secret_version" "anthropic" {
  count       = var.anthropic_api_key == "" ? 0 : 1
  secret      = google_secret_manager_secret.anthropic.id
  secret_data = var.anthropic_api_key
}

resource "google_secret_manager_secret" "stripe_secret_key" {
  project   = var.project_id
  secret_id = "tallyo-${var.env}-stripe-secret-key"

  replication {
    user_managed {
      replicas {
        location = var.region
      }
    }
  }
}

resource "google_secret_manager_secret_version" "stripe_secret_key" {
  count       = var.stripe_secret_key == "" ? 0 : 1
  secret      = google_secret_manager_secret.stripe_secret_key.id
  secret_data = var.stripe_secret_key
}

resource "google_secret_manager_secret" "stripe_webhook_secret" {
  project   = var.project_id
  secret_id = "tallyo-${var.env}-stripe-webhook-secret"

  replication {
    user_managed {
      replicas {
        location = var.region
      }
    }
  }
}

resource "google_secret_manager_secret_version" "stripe_webhook_secret" {
  count       = var.stripe_webhook_secret == "" ? 0 : 1
  secret      = google_secret_manager_secret.stripe_webhook_secret.id
  secret_data = var.stripe_webhook_secret
}
