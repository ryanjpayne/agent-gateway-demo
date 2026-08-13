#!/bin/bash
# Setup prerequisites for AIDR GCP ext_proc shim deployment
#
# This script enables required GCP APIs and validates permissions.
#
# Usage:
#   ./scripts/setup-prerequisites.sh [PROJECT_ID]
#
# If PROJECT_ID is not provided, uses the current gcloud config project.

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
info() { echo -e "${BLUE}INFO:${NC} $1"; }
success() { echo -e "${GREEN}SUCCESS:${NC} $1"; }
warn() { echo -e "${YELLOW}WARNING:${NC} $1"; }
error() { echo -e "${RED}ERROR:${NC} $1"; }

# Required APIs
REQUIRED_APIS=(
    "run.googleapis.com"
    "secretmanager.googleapis.com"
    "cloudbuild.googleapis.com"
    "artifactregistry.googleapis.com"
)

# Check if gcloud is installed
check_gcloud_installed() {
    if ! command -v gcloud &>/dev/null; then
        error "gcloud CLI is not installed."
        echo ""
        echo "Please install the Google Cloud SDK:"
        echo "  https://cloud.google.com/sdk/docs/install"
        echo ""
        echo "Or use Google Cloud Shell which has gcloud pre-installed:"
        echo "  https://shell.cloud.google.com/"
        exit 1
    fi
    success "gcloud CLI is installed"
}

# Check gcloud authentication
check_gcloud_auth() {
    info "Checking gcloud authentication..."
    local account
    account=$(gcloud auth list --filter=status:ACTIVE --format="value(account)" 2>/dev/null | head -n1)

    if [[ -z "$account" ]]; then
        error "Not authenticated with gcloud."
        echo ""
        echo "Please authenticate using one of:"
        echo "  gcloud auth login                    # Interactive browser login"
        echo "  gcloud auth activate-service-account # Service account"
        exit 1
    fi
    success "Authenticated as: $account"
}

# Check if an API is enabled
check_api_enabled() {
    local api="$1"
    local project="$2"
    gcloud services list \
        --project="$project" \
        --enabled \
        --filter="name:$api" \
        --format="value(name)" 2>/dev/null | grep -q "$api"
}

# Enable an API
enable_api() {
    local api="$1"
    local project="$2"
    info "Enabling $api..."
    gcloud services enable "$api" --project="$project"
    success "Enabled $api"
}

# Check IAM permissions
check_permissions() {
    local project="$1"
    info "Checking IAM permissions..."

    # Get current user/service account
    local account
    account=$(gcloud config get-value account 2>/dev/null)

    # Test permissions by checking if we can list services
    if ! gcloud services list --project="$project" --limit=1 &>/dev/null; then
        error "Insufficient permissions to list services in project: $project"
        echo ""
        echo "Required roles (at minimum):"
        echo "  - roles/run.admin                  # Cloud Run Admin"
        echo "  - roles/secretmanager.admin        # Secret Manager Admin"
        echo "  - roles/cloudbuild.builds.editor   # Cloud Build Editor"
        echo ""
        echo "Or use the broader role:"
        echo "  - roles/editor                     # Project Editor"
        return 1
    fi

    success "Basic permissions verified for: $account"
    echo ""
    warn "Note: Full deployment requires these roles:"
    echo "  - roles/run.admin (or roles/run.developer)"
    echo "  - roles/secretmanager.admin (or roles/secretmanager.secretVersionManager)"
    echo "  - roles/cloudbuild.builds.editor"
    echo "  - roles/artifactregistry.writer"
}

# Main function
main() {
    echo ""
    echo "=============================================="
    echo "  AIDR GCP Shim - Prerequisites Setup"
    echo "=============================================="
    echo ""

    # Check gcloud is installed
    check_gcloud_installed
    echo ""

    # Check authentication
    check_gcloud_auth
    echo ""

    # Get project ID
    local project_id="${1:-$(gcloud config get-value project 2>/dev/null || echo "")}"

    if [[ -z "$project_id" ]]; then
        error "No project ID specified and no default project configured."
        echo ""
        echo "Usage: $0 [PROJECT_ID]"
        echo ""
        echo "Or set a default project:"
        echo "  gcloud config set project YOUR_PROJECT_ID"
        exit 1
    fi

    info "Using project: $project_id"
    echo ""

    # Verify project exists and is accessible
    if ! gcloud projects describe "$project_id" &>/dev/null; then
        error "Cannot access project: $project_id"
        echo ""
        echo "Please verify:"
        echo "  1. The project ID is correct"
        echo "  2. You have access to the project"
        echo "  3. Billing is enabled for the project"
        exit 1
    fi
    success "Project is accessible: $project_id"
    echo ""

    # Check and enable APIs
    info "Checking required APIs..."
    local apis_to_enable=()

    for api in "${REQUIRED_APIS[@]}"; do
        if check_api_enabled "$api" "$project_id"; then
            echo -e "  ${GREEN}✓${NC} $api"
        else
            echo -e "  ${RED}✗${NC} $api"
            apis_to_enable+=("$api")
        fi
    done
    echo ""

    # Enable missing APIs
    if [[ ${#apis_to_enable[@]} -gt 0 ]]; then
        warn "Some APIs need to be enabled."
        read -p "Enable missing APIs now? [Y/n]: " enable_confirm

        if [[ "${enable_confirm:-Y}" =~ ^[Yy]$ ]]; then
            echo ""
            for api in "${apis_to_enable[@]}"; do
                enable_api "$api" "$project_id"
            done
            echo ""
            success "All required APIs are now enabled"
        else
            error "Cannot proceed without required APIs."
            exit 1
        fi
    else
        success "All required APIs are already enabled"
    fi
    echo ""

    # Check permissions
    check_permissions "$project_id"
    echo ""

    # Summary
    echo "=============================================="
    echo "  Prerequisites Check Complete"
    echo "=============================================="
    echo ""
    success "Your environment is ready for deployment!"
    echo ""
    echo "Next steps:"
    echo "  1. Run the deployment script:"
    echo "     ./scripts/deploy.sh"
    echo ""
    echo "  2. Or use Terraform for production deployments:"
    echo "     cd terraform && terraform init && terraform apply"
    echo ""
}

# Run main function
main "$@"
