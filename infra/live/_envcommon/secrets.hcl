terraform {
  source = "${get_repo_root()}/infra/modules//secrets"
}

inputs = {
  # Secret VALUES are populated out-of-band (never committed). Leaving them blank
  # creates the Secret Manager secret container with no version; set the version
  # via gcloud/console after apply.
  anthropic_api_key     = ""
  stripe_secret_key     = ""
  stripe_webhook_secret = ""
}
