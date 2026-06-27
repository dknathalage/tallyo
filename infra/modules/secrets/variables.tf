variable "project_id" {
  type = string
}

variable "env" {
  type = string
}

variable "region" {
  type = string
}

variable "anthropic_api_key" {
  type      = string
  default   = ""
  sensitive = true
}

variable "stripe_secret_key" {
  type      = string
  default   = ""
  sensitive = true
}

variable "stripe_webhook_secret" {
  type      = string
  default   = ""
  sensitive = true
}
