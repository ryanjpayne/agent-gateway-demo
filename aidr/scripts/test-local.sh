#!/bin/bash
# Test AIDR ext_proc shim locally via Envoy proxy
#
# Usage: ./scripts/test-local.sh [options]
#
# Options:
#   --url=URL      Envoy listener URL (default: http://localhost:10000)
#   --single=FILE  Test a single payload file
#   --verbose      Show full response headers and body

set -euo pipefail

ENVOY_URL="http://localhost:10000"
SINGLE_FILE=""
VERBOSE=""

for arg in "$@"; do
    case $arg in
        --url=*)
            ENVOY_URL="${arg#*=}"
            ;;
        --single=*)
            SINGLE_FILE="${arg#*=}"
            ;;
        --verbose)
            VERBOSE="-v"
            ;;
        *)
            echo "Unknown argument: $arg"
            exit 1
            ;;
    esac
done

TESTDATA_DIR="$(cd "$(dirname "$0")/../test/testdata" && pwd)"

send_payload() {
    local file="$1"
    local name
    name=$(basename "$file" .json)

    echo "--- $name ---"
    echo "Payload: $(cat "$file")"
    echo ""

    local http_code
    local response
    response=$(curl -s -w "\n%{http_code}" $VERBOSE \
        -X POST "$ENVOY_URL/" \
        -H "Content-Type: application/json" \
        -d @"$file" 2>&1)

    http_code=$(echo "$response" | tail -1)
    body=$(echo "$response" | sed '$d')

    echo "HTTP $http_code"
    echo "Response: $body"

    if [[ "$http_code" == "200" ]]; then
        echo "Result: ALLOWED"
    elif [[ "$http_code" == "403" ]]; then
        echo "Result: BLOCKED"
    else
        echo "Result: UNEXPECTED ($http_code)"
    fi
    echo ""
}

echo "=== Testing AIDR shim via Envoy ==="
echo "Envoy URL: $ENVOY_URL"
echo ""

if [[ -n "$SINGLE_FILE" ]]; then
    send_payload "$SINGLE_FILE"
else
    for file in "$TESTDATA_DIR"/*.json; do
        send_payload "$file"
    done
fi

echo "=== Done ==="
