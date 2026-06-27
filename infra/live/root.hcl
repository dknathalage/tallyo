locals {
  project      = read_terragrunt_config(find_in_parent_folders("project.hcl"))
  region       = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  project_id   = local.project.locals.project_id
  region_id    = local.region.locals.region
  state_bucket = "tallyo-tofu-state" # MUST equal the bucket created in Task 1 (bootstrap state_bucket_name). GCS bucket names are globally unique — if "tallyo-tofu-state" is taken, pick another and use the SAME value in both infra/bootstrap/terraform.tfvars AND here.

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
    }
  EOF
}

inputs = {
  project_id = local.project_id
  region     = local.region_id
}
