output "state_bucket_name" {
  description = "Name of the GCS bucket backing remote tofu state."
  value       = google_storage_bucket.state.name
}

output "enabled_core_apis" {
  description = "Core GCP APIs enabled by the bootstrap."
  value       = sort([for s in google_project_service.core : s.service])
}
