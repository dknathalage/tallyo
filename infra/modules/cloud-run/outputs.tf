output "service_name" {
  description = "Cloud Run service name."
  value       = google_cloud_run_v2_service.this.name
}

output "url" {
  description = "Cloud Run service URL."
  value       = google_cloud_run_v2_service.this.uri
}

output "runtime_service_account" {
  description = "Email of the per-env runtime service account."
  value       = google_service_account.runtime.email
}
