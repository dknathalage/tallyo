terraform {
  required_version = ">= 1.8.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

# GCIP runs on these two APIs: identitytoolkit serves auth, apikeys mints the
# browser key the web SDK ships in its config.
resource "google_project_service" "this" {
  for_each           = toset(["identitytoolkit.googleapis.com", "apikeys.googleapis.com"])
  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

# Project-wide GCIP config (single config per project; all envs share it).
# Providers are enabled here at the platform layer; per-env exposure (which
# methods the app shows/accepts) is a separate app feature flag on Cloud Run.
resource "google_identity_platform_config" "default" {
  project = var.project_id

  sign_in {
    allow_duplicate_emails = false

    email {
      # Email provider backs BOTH email/password and email-link sign-in.
      enabled = var.email_password_enabled || var.email_link_enabled
      # password_required=false is what lets email-link work; keep password
      # sign-in available too. Only force a password when link is off.
      # ponytail: GCIP config can't toggle email-LINK separately from
      # email/password; the app feature flag (AUTH_EMAIL_LINK_ENABLED) gates
      # exposure. Tighten in console if hard separation is ever needed.
      password_required = var.email_password_enabled && !var.email_link_enabled
    }
  }

  authorized_domains = var.authorized_domains

  depends_on = [google_project_service.this]
}

# Google sign-in. client_id/secret come from an OAuth 2.0 client + consent
# screen, which are created manually (no clean TF path) — feed them via vars.
# Skipped entirely until both google is enabled AND the client_id is supplied.
resource "google_identity_platform_default_supported_idp_config" "google" {
  count = var.google_enabled && var.google_client_id != "" ? 1 : 0

  project       = var.project_id
  enabled       = true
  idp_id        = "google.com"
  client_id     = var.google_client_id
  client_secret = var.google_client_secret

  depends_on = [google_identity_platform_config.default]
}

# Browser key for the web SDK config. Firebase/GCIP web API keys are public by
# design (they identify the project, they don't authorize) — restricting to the
# identitytoolkit API is enough. Add browser_key_restrictions referrers later if
# you want to pin it to your domains.
resource "google_apikeys_key" "web" {
  project      = var.project_id
  name         = "tallyo-auth-web"
  display_name = "Tallyo Identity Platform web client"

  restrictions {
    api_targets {
      service = "identitytoolkit.googleapis.com"
    }
  }

  depends_on = [google_project_service.this]
}
