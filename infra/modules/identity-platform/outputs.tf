output "web_api_key" {
  value     = google_apikeys_key.web.key_string
  sensitive = true
}

output "auth_domain" {
  value = "${var.project_id}.firebaseapp.com"
}

output "project_id" {
  value = var.project_id
}
