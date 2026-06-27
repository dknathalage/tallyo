include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "envcommon" {
  path   = "${get_repo_root()}/infra/live/_envcommon/database.hcl"
  expose = true
}

dependency "secrets" {
  config_path = "${dirname(get_terragrunt_dir())}/secrets"

  mock_outputs = {
    db_password = "mock-pw"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan", "init"]
}

inputs = {
  database_name = "tallyo_prd"
  user_name     = "tallyo_prd"
  user_password = dependency.secrets.outputs.db_password
}
