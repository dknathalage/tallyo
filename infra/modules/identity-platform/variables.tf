variable "project_id" {
  type = string
}

variable "authorized_domains" {
  type        = list(string)
  description = "Domains allowed to use GCIP sign-in (OAuth redirect / popup). Include localhost for dev and any custom domains."
  default     = ["localhost"]
}

variable "email_password_enabled" {
  type    = bool
  default = true
}

variable "email_link_enabled" {
  type    = bool
  default = true
}

variable "google_enabled" {
  type    = bool
  default = true
}

variable "google_client_id" {
  type        = string
  description = "OAuth 2.0 client ID for Google sign-in (created manually). Empty disables the Google IdP."
  default     = ""
}

variable "google_client_secret" {
  type      = string
  default   = ""
  sensitive = true
}
