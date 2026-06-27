variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "instance_name" {
  type    = string
  default = "tallyo-pg"
}

variable "database_version" {
  type    = string
  default = "POSTGRES_17"
}

variable "tier" {
  type    = string
  default = "db-f1-micro"
}

variable "disk_size_gb" {
  type    = number
  default = 10
}

variable "deletion_protection" {
  type    = bool
  default = true
}
