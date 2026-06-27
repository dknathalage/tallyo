output "repository_id" {
  description = "Artifact Registry repository id."
  value       = google_artifact_registry_repository.this.repository_id
}

output "repository_name" {
  description = "Fully-qualified Artifact Registry repository resource name."
  value       = google_artifact_registry_repository.this.name
}

output "registry_url" {
  description = "Docker registry URL prefix for image refs."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.this.repository_id}"
}
