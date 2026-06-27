variable "project_id" {
  type = string
}

variable "region" {
  type    = string
  default = "australia-southeast1"
}

variable "state_bucket_name" {
  description = "Globally-unique GCS bucket for tofu state."
  type        = string
}

variable "core_apis" {
  type = list(string)
  default = [
    "serviceusage.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
    "storage.googleapis.com",
  ]
}
