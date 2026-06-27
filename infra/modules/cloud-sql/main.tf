terraform {
  required_version = ">= 1.8.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

resource "google_sql_database_instance" "this" {
  project             = var.project_id
  name                = var.instance_name
  region              = var.region
  database_version    = var.database_version
  deletion_protection = var.deletion_protection

  settings {
    tier              = var.tier
    edition           = "ENTERPRISE" # db-f1-micro shared-core is only valid for ENTERPRISE (not ENTERPRISE_PLUS)
    availability_type = "ZONAL"
    disk_type         = "PD_SSD"
    disk_size         = var.disk_size_gb
    disk_autoresize   = true

    # Public IP, but access is ONLY via the Cloud SQL Auth Proxy (Cloud Run's
    # --add-cloudsql-instances) authenticated by IAM — no authorized_networks,
    # so there is no open network path. The built-in connector needs an IP
    # endpoint; private IP would require a VPC (deliberately avoided for cost).
    ip_configuration {
      ipv4_enabled = true
    }

    backup_configuration {
      enabled = true
    }
  }
}
