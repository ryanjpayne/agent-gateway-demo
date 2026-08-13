variable "project_id" {
  description = "GCP project ID where resources will be created"
  type        = string
}

variable "region" {
  description = "GCP region for Cloud Run deployment"
  type        = string
  default     = "us-central1"
}

variable "service_name" {
  description = "Name for the Cloud Run service"
  type        = string
  default     = "aidr-shim"
}

variable "aidr_cloud" {
  description = "CrowdStrike Falcon cloud region (e.g., us-1, us-2, eu-1, us-gov-1, us-gov-2)"
  type        = string
}

variable "aidr_token" {
  description = "AIDR bearer token for API authentication"
  type        = string
  sensitive   = true
}

variable "min_instances" {
  description = "Minimum number of Cloud Run instances (0 for scale-to-zero)"
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum number of Cloud Run instances"
  type        = number
  default     = 10
}

variable "cpu" {
  description = "CPU allocation for each instance"
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory allocation for each instance"
  type        = string
  default     = "512Mi"
}

variable "log_level" {
  description = "Logging level (debug, info, warn, error)"
  type        = string
  default     = "info"
}

variable "debug_mode" {
  description = "Enable debug mode for verbose logging (not recommended for production)"
  type        = bool
  default     = false
}

variable "collector_instance_id" {
  description = "Optional identifier for this shim instance"
  type        = string
  default     = ""
}

variable "container_image" {
  description = "Container image to deploy (e.g., gcr.io/PROJECT/aidr-shim:latest). Must be built and pushed before applying."
  type        = string
}

variable "allow_unauthenticated" {
  description = "Allow unauthenticated access to Cloud Run service. WARNING: Only enable for development or when service is behind a load balancer with its own auth."
  type        = bool
  default     = false
}
