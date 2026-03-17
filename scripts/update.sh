#!/bin/bash
set -euo pipefail

REPO_DIR="/root/invoices"
DEPLOY_DIR="/opt/invoices"
SERVICE="invoices.service"
LOG_TAG="invoices-updater"

log() { logger -t "$LOG_TAG" "$*"; echo "[$(date -Iseconds)] $*"; }

cd "$REPO_DIR"

# Fetch latest changes
log "Fetching latest changes from origin..."
git fetch origin main --quiet

LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)

if [ "$LOCAL" = "$REMOTE" ]; then
    log "Already up to date ($LOCAL)"
    exit 0
fi

log "Update available: $LOCAL -> $REMOTE"

# Pull changes
git pull origin main --quiet

# Install dependencies if package-lock changed
if ! git diff --quiet "$LOCAL" "$REMOTE" -- package-lock.json; then
    log "package-lock.json changed, running npm ci..."
    npm ci --omit=dev
fi

# Build
log "Building application..."
npm run build

# Deploy
log "Deploying to $DEPLOY_DIR..."
rsync -a --delete build/ "$DEPLOY_DIR/build/"
rsync -a package.json package-lock.json "$DEPLOY_DIR/"

# Copy node_modules if they were reinstalled
if ! git diff --quiet "$LOCAL" "$REMOTE" -- package-lock.json; then
    log "Syncing node_modules..."
    rsync -a --delete node_modules/ "$DEPLOY_DIR/node_modules/"
fi

# Restart service
log "Restarting $SERVICE..."
systemctl restart "$SERVICE"

# Wait and verify
sleep 3
if systemctl is-active --quiet "$SERVICE"; then
    NEW_VERSION=$(node -e "console.log(require('$DEPLOY_DIR/package.json').version)")
    log "Update successful! Now running v$NEW_VERSION"
else
    log "ERROR: Service failed to start after update!"
    systemctl status "$SERVICE" --no-pager || true
    exit 1
fi
