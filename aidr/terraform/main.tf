# Enable required APIs
resource "google_project_service" "required_apis" {
  for_each = toset([
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "cloudbuild.googleapis.com",
    "artifactregistry.googleapis.com",
  ])

  project = var.project_id
  service = each.key

  disable_on_destroy = false
}

# Cloud Run service for AIDR ext_proc shim
resource "google_cloud_run_v2_service" "aidr_shim" {
  name     = var.service_name
  location = var.region
  project  = var.project_id

  # Ensure APIs are enabled and secret IAM bindings exist first
  depends_on = [
    google_project_service.required_apis,
    google_secret_manager_secret_iam_member.aidr_cloud_access,
    google_secret_manager_secret_iam_member.aidr_token_access,
  ]

  template {
    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = var.container_image

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }

      ports {
        container_port = 8080
      }

      # Environment variables
      env {
        name  = "LOG_LEVEL"
        value = var.log_level
      }

      env {
        name  = "DEBUG_MODE"
        value = tostring(var.debug_mode)
      }

      dynamic "env" {
        for_each = var.collector_instance_id != "" ? [1] : []
        content {
          name  = "COLLECTOR_INSTANCE_ID"
          value = var.collector_instance_id
        }
      }

      # Secret references
      env {
        name = "AIDR_CLOUD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.aidr_cloud.secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "AIDR_TOKEN"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.aidr_token.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  deletion_protection = false

  ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"

  labels = {
    app = "aidr-shim"
  }
}

# IAM policy to allow unauthenticated access
resource "google_cloud_run_v2_service_iam_member" "allow_unauthenticated" {
  count    = var.allow_unauthenticated ? 1 : 0
  project  = google_cloud_run_v2_service.aidr_shim.project
  location = google_cloud_run_v2_service.aidr_shim.location
  name     = google_cloud_run_v2_service.aidr_shim.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
