output "enabled_services" {
  description = "Sorted list of enabled GCP service APIs."
  value       = sort([for s in google_project_service.this : s.service])
}
