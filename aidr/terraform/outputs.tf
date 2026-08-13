output "service_url" {
  description = "URL of the deployed Cloud Run service. Provide this to Google for Load Balancer ext_proc configuration."
  value       = google_cloud_run_v2_service.aidr_shim.uri
}

output "service_name" {
  description = "Name of the deployed Cloud Run service"
  value       = google_cloud_run_v2_service.aidr_shim.name
}

output "service_location" {
  description = "Location (region) of the deployed Cloud Run service"
  value       = google_cloud_run_v2_service.aidr_shim.location
}

output "project_id" {
  description = "GCP project ID where resources were created"
  value       = var.project_id
}

output "secret_ids" {
  description = "Secret Manager secret IDs for AIDR credentials"
  value = {
    cloud = google_secret_manager_secret.aidr_cloud.secret_id
    token = google_secret_manager_secret.aidr_token.secret_id
  }
}
