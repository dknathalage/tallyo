locals {
  project      = read_terragrunt_config(find_in_parent_folders("project.hcl"))
  region       = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  project_id   = local.project.locals.project_id
  region_id    = local.region.locals.region
  state_bucket = "tallyo-saas-tofu-state" # created by infra/bootstrap; globally unique

  # Credential-free validation: set TG_OFFLINE=1 to swap the GCS backend for a
  # local one so `terragrunt run --all validate` works without GCP credentials.
  # Real plan/apply leaves this unset and uses GCS remote state.
  offline = get_env("TG_OFFLINE", "") != ""
}

remote_state {
  backend = local.offline ? "local" : "gcs"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = local.offline ? {
    path = "${get_terragrunt_dir()}/offline.tfstate"
    } : {
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
    provider "google" {
      project = "${local.project_id}"
      region  = "${local.region_id}"

      # identitytoolkit / apikeys require a quota project on the request. With
      # user (ADC) creds this header is only sent when user_project_override is
      # on; billing_project names the project to bill the quota to.
      user_project_override = true
      billing_project       = "${local.project_id}"
    }
  EOF
}

inputs = {
  project_id = local.project_id
  region     = local.region_id
}
