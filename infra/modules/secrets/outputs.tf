output "db_password" {
  description = "Generated DB password for this env."
  value       = random_password.db.result
  sensitive   = true
}

output "db_password_secret_id" {
  description = "Secret Manager secret id holding the DB password."
  value       = google_secret_manager_secret.db_password.secret_id
}

output "anthropic_secret_id" {
  description = "Secret Manager secret id holding the ANTHROPIC_API_KEY."
  value       = google_secret_manager_secret.anthropic.secret_id
}
