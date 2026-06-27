terraform {
  source = "${get_repo_root()}/infra/modules//secrets"
}

inputs = {
  anthropic_api_key = ""
}
