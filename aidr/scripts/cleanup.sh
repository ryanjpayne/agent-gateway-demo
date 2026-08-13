#!/bin/bash
# Clean up GCP resources created by the AIDR ext_proc shim deploy scripts
#
# Usage:
#   Interactive: ./scripts/cleanup.sh
#   Non-interactive: CONFIRM=true ./scripts/cleanup.sh
#
# Environment variables:
#   PROJECT_ID   - GCP project ID (defaults to current gcloud config)
#   REGION       - Cloud Run region (default: us-central1)
#   CONFIRM      - Skip confirmation prompt (default: false)

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

PROJECT_ID="${PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || echo "")}"
REGION="${REGION:-us-central1}"
CONFIRM="${CONFIRM:-false}"

if [[ -z "$PROJECT_ID" ]]; then
    error "PROJECT_ID not set and no default project configured"
    exit 1
fi

echo ""
echo "=============================================="
echo "  AIDR GCP ext_proc Shim Cleanup"
echo "=============================================="
echo ""

info "Project: $PROJECT_ID"
info "Region: $REGION"
echo ""

# Discover what exists
services=()
for name in aidr-shim aidr-shim-debug; do
    if gcloud run services describe "$name" --region="$REGION" --project="$PROJECT_ID" &>/dev/null; then
        services+=("$name")
    fi
done

secrets=()
for name in aidr-cloud aidr-token; do
    if gcloud secrets describe "$name" --project="$PROJECT_ID" &>/dev/null; then
        secrets+=("$name")
    fi
done

if [[ ${#services[@]} -eq 0 && ${#secrets[@]} -eq 0 ]]; then
    info "No AIDR resources found to clean up."
    exit 0
fi

# Show what will be deleted
warn "The following resources will be deleted:"
echo ""
for svc in "${services[@]}"; do
    echo "  Cloud Run service: $svc ($REGION)"
done
for secret in "${secrets[@]}"; do
    echo "  Secret Manager secret: $secret"
done
echo ""

# Confirm
if [[ "$CONFIRM" != "true" ]]; then
    read -p "Are you sure you want to delete these resources? [y/N]: " answer
    if [[ ! "${answer}" =~ ^[Yy]$ ]]; then
        info "Cleanup cancelled."
        exit 0
    fi
    echo ""
fi

# Delete Cloud Run services
for svc in "${services[@]}"; do
    info "Deleting Cloud Run service: $svc"
    gcloud run services delete "$svc" \
        --region="$REGION" \
        --project="$PROJECT_ID" \
        --quiet
    success "Deleted $svc"
done

# Delete secrets
for secret in "${secrets[@]}"; do
    info "Deleting secret: $secret"
    gcloud secrets delete "$secret" \
        --project="$PROJECT_ID" \
        --quiet
    success "Deleted $secret"
done

echo ""
echo "=============================================="
echo "  Cleanup Complete"
echo "=============================================="
echo ""
success "All AIDR resources have been removed from project $PROJECT_ID"
echo ""
