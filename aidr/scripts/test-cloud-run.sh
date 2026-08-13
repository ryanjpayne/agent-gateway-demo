#!/bin/bash
# Test deployed AIDR ext_proc shim on Cloud Run with test payloads
#
# Usage: ./scripts/test-cloud-run.sh [options]
#
# Options:
#   --service=NAME   Service name (default: aidr-shim-debug)
#   --region=REGION  Cloud Run region (default: us-central1)
#   --single=FILE    Test a single payload file instead of batch
#
# Environment variables:
#   PROJECT_ID - GCP project ID (defaults to current gcloud config)

set -euo pipefail

# Default values
SERVICE_NAME="aidr-shim-debug"
REGION="us-central1"
SINGLE_FILE=""

# Parse arguments
for arg in "$@"; do
    case $arg in
        --service=*)
            SERVICE_NAME="${arg#*=}"
            ;;
        --region=*)
            REGION="${arg#*=}"
            ;;
        --single=*)
            SINGLE_FILE="${arg#*=}"
            ;;
        *)
            echo "Unknown argument: $arg"
            exit 1
            ;;
    esac
done

PROJECT_ID=${PROJECT_ID:-$(gcloud config get-value project 2>/dev/null)}

if [[ -z "$PROJECT_ID" ]]; then
    echo "Error: PROJECT_ID not set and no default project configured"
    exit 1
fi

# Get service URL
SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" \
    --project="$PROJECT_ID" \
    --region="$REGION" \
    --format='value(status.url)' 2>/dev/null)

if [[ -z "$SERVICE_URL" ]]; then
    echo "Error: Could not find service $SERVICE_NAME in region $REGION"
    echo "Run deploy-debug.sh first to deploy the service"
    exit 1
fi

# Extract host from URL (remove https://)
SERVICE_HOST="${SERVICE_URL#https://}"

echo "=== Testing AIDR ext_proc shim on Cloud Run ==="
echo "Service: $SERVICE_NAME"
echo "URL: $SERVICE_URL"
echo "Host: $SERVICE_HOST"
echo ""

if [[ -n "$SINGLE_FILE" ]]; then
    # Test single file
    echo "Testing single payload: $SINGLE_FILE"
    echo ""
    go run ./test/client \
        --address="$SERVICE_HOST:443" \
        --tls \
        --payload="$SINGLE_FILE"
else
    # Batch test all payloads
    echo "Running batch tests from test/testdata/"
    echo ""
    go run ./test/client \
        --address="$SERVICE_HOST:443" \
        --tls \
        --batch \
        --testdata=test/testdata/
fi

echo ""
echo "=== Test complete ==="
echo ""
echo "To view logs:"
echo "  gcloud run services logs read $SERVICE_NAME --region=$REGION --project=$PROJECT_ID --limit=50"
