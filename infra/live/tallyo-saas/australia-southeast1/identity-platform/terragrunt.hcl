include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "envcommon" {
  path   = "${get_repo_root()}/infra/live/_envcommon/identity-platform.hcl"
  expose = true
}

inputs = {
  # GCIP config is project-wide (single config shared by all envs), so this
  # leaf lives at the region level, not per-env.

  # Domains allowed to use GCIP sign-in (OAuth redirect / popup / email-link).
  # Cloud Run service hosts; add a custom domain here when you have one.
  # Cloud Run serves two URL schemes per service; authorize both.
  authorized_domains = [
    "localhost",
    "tallyo-prd-xtjnnjgk6a-ts.a.run.app",
    "tallyo-stg-xtjnnjgk6a-ts.a.run.app",
    "tallyo-dev-xtjnnjgk6a-ts.a.run.app",
    "tallyo-prd-423893456095.australia-southeast1.run.app",
    "tallyo-stg-423893456095.australia-southeast1.run.app",
    "tallyo-dev-423893456095.australia-southeast1.run.app",
  ]

  # Google sign-in. The client_id is public (it ships in the browser OAuth flow),
  # so it's safe to commit. The secret is read from the GOOGLE_OAUTH_CLIENT_SECRET
  # env var at apply time and is NEVER committed. Both come from the OAuth 2.0
  # Web client created in the console (redirect URI:
  # https://tallyo-saas.firebaseapp.com/__/auth/handler).
  google_client_id     = "423893456095-7iqcauiflbks0qvdsec9u5bmrv35jq0j.apps.googleusercontent.com"
  google_client_secret = get_env("GOOGLE_OAUTH_CLIENT_SECRET", "")
}
