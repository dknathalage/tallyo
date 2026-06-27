output "database_name" {
  description = "Created database name."
  value       = google_sql_database.this.name
}

output "user_name" {
  description = "Created database user name."
  value       = google_sql_user.this.name
}
