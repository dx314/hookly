#!/bin/bash
set -e

# Hookly Edge Gateway Deploy Script
# Usage: ./deploy.sh [--build-only|--push-only|--deploy-only]

REGISTRY="git.dev.alexdunmow.com"
IMAGE="$REGISTRY/alex/hookly/edge:latest"
COOLIFY_URL="https://svr.alexdunmow.com"
APP_UUID="w80kgwc0wckwswowswk0k8cs"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() { echo -e "${GREEN}==>${NC} $1"; }
warn() { echo -e "${YELLOW}==>${NC} $1"; }
error() { echo -e "${RED}==>${NC} $1"; exit 1; }

# Check for Coolify API token
if [[ -z "$COOLIFY_TOKEN" ]]; then
    if [[ -f ~/.config/coolify/token ]]; then
        COOLIFY_TOKEN=$(cat ~/.config/coolify/token)
    else
        error "COOLIFY_TOKEN not set. Export it or save to ~/.config/coolify/token"
    fi
fi

build() {
    log "Building Docker image..."
    cd "$(dirname "$0")/../.."
    docker build -t "$IMAGE" -f deploy/edge/Dockerfile .
    log "Build complete: $IMAGE"
}

push() {
    log "Pushing to registry..."
    docker push "$IMAGE"
    log "Push complete"
}

deploy() {
    log "Triggering Coolify deployment..."
    RESPONSE=$(curl -s -H "Authorization: Bearer $COOLIFY_TOKEN" \
        "$COOLIFY_URL/api/v1/deploy?uuid=$APP_UUID")

    if echo "$RESPONSE" | grep -q "deployment_uuid"; then
        DEPLOY_UUID=$(echo "$RESPONSE" | jq -r '.deployments[0].deployment_uuid')
        log "Deployment queued: $DEPLOY_UUID"

        # Wait for deployment
        log "Waiting for deployment to complete..."
        for i in {1..60}; do
            STATUS=$(curl -s -H "Authorization: Bearer $COOLIFY_TOKEN" \
                "$COOLIFY_URL/api/v1/deployments/$DEPLOY_UUID" | jq -r '.status')

            if [[ "$STATUS" == "finished" ]]; then
                log "Deployment finished successfully!"
                break
            elif [[ "$STATUS" == "failed" ]]; then
                error "Deployment failed! Check Coolify logs."
            fi
            sleep 2
        done

        # Verify health
        log "Verifying health..."
        sleep 3
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" https://hooks.dx314.com/health)
        if [[ "$HTTP_CODE" == "200" ]]; then
            log "Health check passed! App is live at https://hooks.dx314.com"
        else
            warn "Health check returned $HTTP_CODE - check logs"
        fi
    else
        error "Failed to queue deployment: $RESPONSE"
    fi
}

# Parse arguments
case "${1:-}" in
    --build-only)
        build
        ;;
    --push-only)
        push
        ;;
    --deploy-only)
        deploy
        ;;
    *)
        build
        push
        deploy
        ;;
esac
