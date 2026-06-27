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

variable "stripe_secret_key_secret_id" {
  type = string
}

variable "stripe_webhook_secret_secret_id" {
  type = string
}

variable "billing_enabled" {
  type    = bool
  default = false
}

variable "stripe_price_id" {
  type    = string
  default = ""
}

variable "stripe_price_id_annual" {
  type    = string
  default = ""
}

variable "trial_days" {
  type    = number
  default = 30
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

variable "firebase_api_key" {
  type = string
}

variable "firebase_auth_domain" {
  type = string
}

variable "firebase_project_id" {
  type = string
}

variable "auth_email_password_enabled" {
  type    = bool
  default = true
}

variable "auth_google_enabled" {
  type    = bool
  default = true
}

variable "auth_email_link_enabled" {
  type    = bool
  default = true
}

variable "invoker_members" {
  type        = list(string)
  description = "Members granted roles/run.invoker when the service is not public (e.g. [\"user:dknathalage@gmail.com\"])."
  default     = []
}
