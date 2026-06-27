variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "repository_id" {
  type    = string
  default = "tallyo"
}

variable "description" {
  type    = string
  default = "Tallyo container images"
}
