include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "envcommon" {
  path   = "${get_repo_root()}/infra/live/_envcommon/cloud-run.hcl"
  expose = true
}

inputs = {
  env          = "dev"
  service_name = "tallyo-dev"
}
