# Secret Manager secrets for AIDR credentials

resource "google_secret_manager_secret" "aidr_cloud" {
  secret_id = "aidr-cloud"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = {
    app = "aidr-shim"
  }
}

resource "google_secret_manager_secret_version" "aidr_cloud" {
  secret      = google_secret_manager_secret.aidr_cloud.id
  secret_data = var.aidr_cloud
}

resource "google_secret_manager_secret" "aidr_token" {
  secret_id = "aidr-token"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = {
    app = "aidr-shim"
  }
}

resource "google_secret_manager_secret_version" "aidr_token" {
  secret      = google_secret_manager_secret.aidr_token.id
  secret_data = var.aidr_token
}

# Look up project number to construct the default compute service account
data "google_project" "current" {
  project_id = var.project_id
}

# IAM binding for Cloud Run service account to access secrets
# Must be created BEFORE the Cloud Run service so it can read secrets on startup
resource "google_secret_manager_secret_iam_member" "aidr_cloud_access" {
  secret_id = google_secret_manager_secret.aidr_cloud.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

resource "google_secret_manager_secret_iam_member" "aidr_token_access" {
  secret_id = google_secret_manager_secret.aidr_token.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}
