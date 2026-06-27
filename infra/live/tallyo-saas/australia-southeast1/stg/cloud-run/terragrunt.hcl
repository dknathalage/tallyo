include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "envcommon" {
  path   = "${get_repo_root()}/infra/live/_envcommon/cloud-run.hcl"
  expose = true
}

inputs = {
  env          = "stg"
  service_name = "tallyo-stg"

  # Edge lockdown: not public; only allowlisted identities can invoke.
  allow_public    = false
  invoker_members = ["user:dknathalage@gmail.com"]

  # Auth methods exposed by the app. Explicit so they're easy to flip per env.
  auth_email_password_enabled = true
  auth_google_enabled         = true
  auth_email_link_enabled     = true
}
