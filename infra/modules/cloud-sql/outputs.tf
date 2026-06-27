output "instance_name" {
  description = "Cloud SQL instance name."
  value       = google_sql_database_instance.this.name
}

output "connection_name" {
  description = "Cloud SQL connection name (PROJECT:REGION:INSTANCE) for the socket DSN and --add-cloudsql-instances."
  value       = google_sql_database_instance.this.connection_name
}
