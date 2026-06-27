variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "env" {
  type = string
}

variable "service_name" {
  type = string
}

variable "image" {
  type = string
}

variable "cloudsql_connection_name" {
  type = string
}

variable "db_name" {
  type = string
}

variable "db_user" {
  type = string
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "anthropic_secret_id" {
  type = string
}

variable "db_password_secret_id" {
  type = string
}

variable "min_instances" {
  type    = number
  default = 0
}

variable "max_instances" {
  type    = number
  default = 3
}

variable "concurrency" {
  type    = number
  default = 80
}

variable "cpu" {
  type    = string
  default = "1"
}

variable "memory" {
  type    = string
  default = "512Mi"
}

variable "allow_public" {
  type    = bool
  default = true
}
