terraform {
  required_version = ">= 1.8.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

resource "google_sql_database" "this" {
  project  = var.project_id
  instance = var.instance_name
  name     = var.database_name
}

resource "google_sql_user" "this" {
  project  = var.project_id
  instance = var.instance_name
  name     = var.user_name
  password = var.user_password
}
