#!/bin/bash
# Production deployment script for AIDR GCP ext_proc shim to Cloud Run
#
# Usage:
#   Interactive: ./scripts/deploy.sh
#   With debug:  ./scripts/deploy.sh --debug
#   Non-interactive: AIDR_CLOUD=... AIDR_TOKEN=... ./scripts/deploy.sh
#
# This script will:
#   1. Validate prerequisites (APIs, permissions, authentication)
#   2. Create secrets in Secret Manager (if they don't exist)
#   3. Deploy the shim to Cloud Run
#   4. Output the service URL for Google Load Balancer configuration
#
# Environment variables (for non-interactive mode):
#   PROJECT_ID      - GCP project ID (defaults to current gcloud config)
#   REGION          - Cloud Run region (default: us-central1)
#   SERVICE_NAME    - Cloud Run service name (default: aidr-shim)
#   AIDR_CLOUD      - Falcon cloud region (e.g. us-1, us-2, eu-1) (required)
#   AIDR_TOKEN      - AIDR bearer token (required)
#   MIN_INSTANCES   - Minimum instances (default: 0)
#   MAX_INSTANCES   - Maximum instances (default: 10)

set -euo pipefail

# Parse flags
DEBUG_MODE_FLAG=false
for arg in "$@"; do
    case "$arg" in
        --debug) DEBUG_MODE_FLAG=true ;;
    esac
done

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

# Configuration defaults
DEFAULT_REGION="us-central1"
DEFAULT_SERVICE_NAME="aidr-shim"
if [[ "$DEBUG_MODE_FLAG" == "true" ]]; then
    DEFAULT_SERVICE_NAME="aidr-shim-debug"
fi
DEFAULT_MIN_INSTANCES="0"
DEFAULT_MAX_INSTANCES="10"

# Interactive prompt function
prompt() {
    local var_name="$1"
    local prompt_text="$2"
    local default_value="${3:-}"
    local is_secret="${4:-false}"

    # Check if already set via environment
    local current_value="${!var_name:-}"
    if [[ -n "$current_value" ]]; then
        if [[ "$is_secret" == "true" ]]; then
            echo "$prompt_text [using environment variable]: ****"
        else
            echo "$prompt_text [using environment variable]: $current_value"
        fi
        return
    fi

    # Interactive prompt
    local input
    if [[ "$is_secret" == "true" ]]; then
        if [[ -n "$default_value" ]]; then
            read -sp "$prompt_text [default: ****]: " input
        else
            read -sp "$prompt_text: " input
        fi
        echo ""
    else
        if [[ -n "$default_value" ]]; then
            read -p "$prompt_text [default: $default_value]: " input
        else
            read -p "$prompt_text: " input
        fi
    fi

    # Use default if empty
    if [[ -z "$input" && -n "$default_value" ]]; then
        input="$default_value"
    fi

    export "$var_name"="$input"
}

# Validation functions
check_gcloud_auth() {
    info "Checking gcloud authentication..."
    if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" 2>/dev/null | head -n1 | grep -q .; then
        error "Not authenticated with gcloud. Please run: gcloud auth login"
        return 1
    fi
    success "gcloud is authenticated"
}

check_api_enabled() {
    local api="$1"
    local project="$2"
    if gcloud services list --project="$project" --enabled --filter="name:$api" --format="value(name)" 2>/dev/null | grep -q "$api"; then
        return 0
    fi
    return 1
}

validate_prerequisites() {
    local project="$1"
    local missing_apis=()

    info "Validating prerequisites for project: $project"

    # Check required APIs
    local required_apis=("run.googleapis.com" "secretmanager.googleapis.com" "cloudbuild.googleapis.com" "artifactregistry.googleapis.com")

    for api in "${required_apis[@]}"; do
        if ! check_api_enabled "$api" "$project"; then
            missing_apis+=("$api")
        fi
    done

    if [[ ${#missing_apis[@]} -gt 0 ]]; then
        warn "The following APIs are not enabled:"
        for api in "${missing_apis[@]}"; do
            echo "  - $api"
        done
        echo ""
        read -p "Would you like to enable them now? [Y/n]: " enable_apis
        if [[ "${enable_apis:-Y}" =~ ^[Yy]$ ]]; then
            for api in "${missing_apis[@]}"; do
                info "Enabling $api..."
                gcloud services enable "$api" --project="$project"
            done
            success "APIs enabled successfully"
        else
            error "Required APIs are not enabled. Please run: ./scripts/setup-prerequisites.sh"
            return 1
        fi
    else
        success "All required APIs are enabled"
    fi
}

create_or_update_secret() {
    local project="$1"
    local secret_name="$2"
    local secret_value="$3"

    # Check if secret exists
    if gcloud secrets describe "$secret_name" --project="$project" &>/dev/null; then
        info "Updating existing secret: $secret_name"
        echo -n "$secret_value" | gcloud secrets versions add "$secret_name" \
            --project="$project" \
            --data-file=-
    else
        info "Creating new secret: $secret_name"
        echo -n "$secret_value" | gcloud secrets create "$secret_name" \
            --project="$project" \
            --data-file=-
    fi
}

# Main deployment flow
main() {
    echo ""
    echo "=============================================="
    echo "  AIDR GCP ext_proc Shim Deployment"
    echo "=============================================="
    echo ""

    # Step 1: Check gcloud authentication
    check_gcloud_auth

    # Step 2: Get project ID
    PROJECT_ID="${PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || echo "")}"
    prompt PROJECT_ID "GCP Project ID" "$PROJECT_ID"

    if [[ -z "$PROJECT_ID" ]]; then
        error "PROJECT_ID is required"
        exit 1
    fi

    # Set project
    gcloud config set project "$PROJECT_ID" 2>/dev/null

    # Step 3: Validate prerequisites
    echo ""
    validate_prerequisites "$PROJECT_ID"

    # Step 4: Get deployment configuration
    echo ""
    info "Deployment configuration:"
    prompt REGION "Cloud Run region" "$DEFAULT_REGION"
    prompt SERVICE_NAME "Service name" "$DEFAULT_SERVICE_NAME"
    prompt MIN_INSTANCES "Minimum instances" "$DEFAULT_MIN_INSTANCES"
    prompt MAX_INSTANCES "Maximum instances" "$DEFAULT_MAX_INSTANCES"

    # Step 5: Get AIDR credentials
    echo ""
    info "AIDR API credentials:"
    prompt AIDR_CLOUD "Falcon cloud region (e.g., us-1, us-2, eu-1, us-gov-1, us-gov-2)"
    prompt AIDR_TOKEN "AIDR bearer token" "" "true"

    if [[ -z "$AIDR_CLOUD" ]]; then
        error "AIDR_CLOUD is required"
        exit 1
    fi

    if [[ -z "$AIDR_TOKEN" ]]; then
        error "AIDR_TOKEN is required"
        exit 1
    fi

    # Step 6: Create/update secrets
    echo ""
    info "Setting up Secret Manager secrets..."
    create_or_update_secret "$PROJECT_ID" "aidr-cloud" "$AIDR_CLOUD"
    create_or_update_secret "$PROJECT_ID" "aidr-token" "$AIDR_TOKEN"
    success "Secrets configured"

    # Step 6b: Grant Cloud Run service account access to secrets
    info "Granting Secret Manager access to Cloud Run service account..."
    local project_number
    project_number=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')
    local sa="${project_number}-compute@developer.gserviceaccount.com"

    for secret in aidr-cloud aidr-token; do
        gcloud secrets add-iam-policy-binding "$secret" \
            --project="$PROJECT_ID" \
            --member="serviceAccount:${sa}" \
            --role="roles/secretmanager.secretAccessor" \
            --quiet
    done
    success "Secret Manager access granted to ${sa}"

    # Step 7: Deploy to Cloud Run
    echo ""
    info "Deploying to Cloud Run..."
    echo "  Project: $PROJECT_ID"
    echo "  Region: $REGION"
    echo "  Service: $SERVICE_NAME"
    echo "  Min instances: $MIN_INSTANCES"
    echo "  Max instances: $MAX_INSTANCES"
    echo ""

    gcloud run deploy "$SERVICE_NAME" \
        --project="$PROJECT_ID" \
        --source . \
        --region="$REGION" \
        --set-env-vars="LOG_LEVEL=$(if [[ "$DEBUG_MODE_FLAG" == "true" ]]; then echo debug; else echo info; fi),DEBUG_MODE=$DEBUG_MODE_FLAG" \
        --set-secrets="AIDR_CLOUD=aidr-cloud:latest,AIDR_TOKEN=aidr-token:latest" \
        --allow-unauthenticated \
        --port=8080 \
        --cpu=1 \
        --memory=512Mi \
        --min-instances="$MIN_INSTANCES" \
        --max-instances="$MAX_INSTANCES" \
        --use-http2

    # Step 8: Get and display service URL
    echo ""
    SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" \
        --project="$PROJECT_ID" \
        --region="$REGION" \
        --format='value(status.url)')

    echo ""
    echo "=============================================="
    echo "  Deployment Complete!"
    echo "=============================================="
    echo ""
    success "Service deployed successfully"
    echo ""
    echo "Service URL (provide this to Google for Load Balancer configuration):"
    echo ""
    echo -e "  ${GREEN}${SERVICE_URL}${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Provide the service URL to Google for Load Balancer ext_proc configuration"
    echo "  2. Configure your AIDR policies in the CrowdStrike console"
    echo ""
    echo "Useful commands:"
    echo "  View logs:"
    echo "    gcloud run services logs read $SERVICE_NAME --region=$REGION --project=$PROJECT_ID --limit=50"
    echo ""
    echo "  Test the deployment:"
    echo "    go run ./test/client --address=${SERVICE_URL#https://}:443 --tls --payload=test/testdata/clean_request.json"
    echo ""
}

# Run main function
main "$@"
