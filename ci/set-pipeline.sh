#!/usr/bin/env bash
# Script to set the Concourse pipeline

set -euo pipefail

# Configuration
PIPELINE_NAME="api-cache"
PIPELINE_FILE="ci/pipeline.yml"
CREDENTIALS_FILE="ci/credentials.yml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_info()    { echo -e "${GREEN}[INFO]${NC} $1"; }
print_error()   { echo -e "${RED}[ERROR]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }

# Check if fly is installed
if ! command -v fly &> /dev/null; then
    print_error "fly CLI is not installed. Please install it first."
    print_info "Visit: https://concourse-ci.org/download.html"
    exit 1
fi

# Check if credentials file exists
if [ ! -f "$CREDENTIALS_FILE" ]; then
    print_error "Credentials file not found: $CREDENTIALS_FILE"
    print_info "Copy the example file and fill in your credentials:"
    print_info "  cp ci/credentials.yml.example ci/credentials.yml"
    exit 1
fi

# Get Concourse target
if [ $# -eq 0 ]; then
    print_error "Please provide a Concourse target name"
    print_info "Usage: $0 <target-name>"
    print_info "Example: $0 deploy"
    print_info ""
    print_info "Available targets:"
    fly targets 2>/dev/null || echo "  (none configured)"
    exit 1
fi

TARGET="$1"

# Verify target exists
if ! fly -t "$TARGET" status &> /dev/null; then
    print_error "Target '$TARGET' is not configured or not logged in"
    print_info "Login first with: fly -t $TARGET login -c https://deploy.gsingh.io"
    exit 1
fi

print_info "Setting pipeline '$PIPELINE_NAME' on target '$TARGET'"

# Set the pipeline
if fly -t "$TARGET" set-pipeline \
    -p "$PIPELINE_NAME" \
    -c "$PIPELINE_FILE" \
    -l "$CREDENTIALS_FILE"; then

    print_info "Pipeline set successfully!"

    read -p "Do you want to unpause the pipeline? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        fly -t "$TARGET" unpause-pipeline -p "$PIPELINE_NAME"
        print_info "Pipeline unpaused!"
    fi

    read -p "Do you want to expose the pipeline (make it public)? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        fly -t "$TARGET" expose-pipeline -p "$PIPELINE_NAME"
        print_info "Pipeline exposed!"
    fi

    print_info "Pipeline URL: https://deploy.gsingh.io/teams/main/pipelines/$PIPELINE_NAME"

else
    print_error "Failed to set pipeline"
    exit 1
fi
