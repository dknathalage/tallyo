terraform {
  source = "${get_repo_root()}/infra/modules//cloud-sql"
}

inputs = {
  instance_name = "tallyo-pg"
  tier          = "db-f1-micro"
}
