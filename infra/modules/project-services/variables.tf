variable "project_id" {
  type = string
}

variable "services" {
  type = list(string)
  default = [
    "run.googleapis.com",
    "sqladmin.googleapis.com",
    "artifactregistry.googleapis.com",
    "secretmanager.googleapis.com",
    "iam.googleapis.com",
    "compute.googleapis.com",
  ]
}
