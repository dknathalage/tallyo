variable "project_id" {
  type = string
}

variable "instance_name" {
  type = string
}

variable "database_name" {
  type = string
}

variable "user_name" {
  type = string
}

variable "user_password" {
  type      = string
  sensitive = true
}
