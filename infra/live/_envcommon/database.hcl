terraform {
  source = "${get_repo_root()}/infra/modules//database"
}

dependency "cloud_sql" {
  config_path = "${dirname(dirname(get_terragrunt_dir()))}/cloud-sql"

  mock_outputs = {
    instance_name   = "tallyo-pg-mock"
    connection_name = "mock:mock:mock"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan", "init"]
}

inputs = {
  instance_name = dependency.cloud_sql.outputs.instance_name
}
