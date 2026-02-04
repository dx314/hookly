#!/bin/bash
set -e

# Hookly Edge Gateway Deploy Script
# Usage: ./deploy.sh [--build-only|--push-only|--deploy-only|--cleanup-only]

REGISTRY="git.dev.alexdunmow.com"
IMAGE="$REGISTRY/alex/hookly/edge:latest"
COOLIFY_URL="https://svr.alexdunmow.com"
APP_UUID="w80kgwc0wckwswowswk0k8cs"
GITEA_OWNER="alex"
GITEA_PACKAGE="hookly/edge"
KEEP_VERSIONS=3  # Number of recent versions to keep

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

# Check for Gitea API token
if [[ -z "$GITEA_TOKEN" ]]; then
    if [[ -f ~/.config/gitea/token ]]; then
        GITEA_TOKEN=$(cat ~/.config/gitea/token)
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

    # Clean up local dangling images
    log "Pruning local dangling images..."
    docker image prune -f --filter "dangling=true" >/dev/null 2>&1 || true
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

cleanup() {
    if [[ -z "$GITEA_TOKEN" ]]; then
        warn "GITEA_TOKEN not set - skipping registry cleanup"
        warn "Set it via env or save to ~/.config/gitea/token"
        return 0
    fi

    log "Cleaning up old container images from Gitea registry..."

    # Get all versions of the package
    VERSIONS=$(curl -s -H "Authorization: token $GITEA_TOKEN" \
        "https://$REGISTRY/api/v1/packages/$GITEA_OWNER/container/$GITEA_PACKAGE" \
        | jq -r '.[].version' 2>/dev/null || echo "")

    if [[ -z "$VERSIONS" ]]; then
        log "No old versions to clean up"
        return 0
    fi

    # Count versions
    VERSION_COUNT=$(echo "$VERSIONS" | wc -l)

    if [[ $VERSION_COUNT -le $KEEP_VERSIONS ]]; then
        log "Only $VERSION_COUNT versions exist, keeping all (threshold: $KEEP_VERSIONS)"
        return 0
    fi

    # Get versions to delete (all except the most recent N)
    # Gitea returns versions sorted by creation date (newest first)
    TO_DELETE=$(echo "$VERSIONS" | tail -n +$((KEEP_VERSIONS + 1)))
    DELETE_COUNT=$(echo "$TO_DELETE" | wc -l)

    log "Found $VERSION_COUNT versions, deleting $DELETE_COUNT old versions..."

    for VERSION in $TO_DELETE; do
        log "  Deleting version: $VERSION"
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
            -H "Authorization: token $GITEA_TOKEN" \
            "https://$REGISTRY/api/v1/packages/$GITEA_OWNER/container/$GITEA_PACKAGE/$VERSION")

        if [[ "$HTTP_CODE" == "204" ]]; then
            log "    Deleted successfully"
        else
            warn "    Failed to delete (HTTP $HTTP_CODE)"
        fi
    done

    log "Cleanup complete"
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
    --cleanup-only)
        cleanup
        ;;
    *)
        build
        push
        deploy
        cleanup
        ;;
esac
