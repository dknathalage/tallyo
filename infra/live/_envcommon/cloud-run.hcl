terraform {
  source = "${get_repo_root()}/infra/modules//cloud-run"
}

locals {
  project    = read_terragrunt_config(find_in_parent_folders("project.hcl"))
  region     = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  project_id = local.project.locals.project_id
  region_id  = local.region.locals.region
  image      = "${local.region_id}-docker.pkg.dev/${local.project_id}/tallyo/tallyo:latest"
}

dependency "cloud_sql" {
  config_path = "${dirname(dirname(get_terragrunt_dir()))}/cloud-sql"

  mock_outputs = {
    instance_name   = "tallyo-pg-mock"
    connection_name = "mock:mock:mock"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan", "init"]
}

dependency "database" {
  config_path = "${dirname(get_terragrunt_dir())}/database"

  mock_outputs = {
    database_name = "tallyo_mock"
    user_name     = "tallyo_mock"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan", "init"]
}

dependency "secrets" {
  config_path = "${dirname(get_terragrunt_dir())}/secrets"

  mock_outputs = {
    db_password           = "mock-pw"
    db_password_secret_id = "tallyo-mock-db-password"
    anthropic_secret_id   = "tallyo-mock-anthropic"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan", "init"]
}

inputs = {
  image                    = local.image
  cloudsql_connection_name = dependency.cloud_sql.outputs.connection_name
  db_name                  = dependency.database.outputs.database_name
  db_user                  = dependency.database.outputs.user_name
  db_password              = dependency.secrets.outputs.db_password
  anthropic_secret_id      = dependency.secrets.outputs.anthropic_secret_id
  db_password_secret_id    = dependency.secrets.outputs.db_password_secret_id
}
