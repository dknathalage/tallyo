#!/bin/bash
set -euo pipefail

REPO_DIR="/root/invoices"
DEPLOY_DIR="/opt/invoices"
SERVICE="invoices.service"
LOG_TAG="invoices-updater"

log() { logger -t "$LOG_TAG" "$*"; echo "[$(date -Iseconds)] $*"; }

cd "$REPO_DIR"

# Fetch latest changes including tags
log "Fetching latest changes from origin..."
git fetch origin --tags --quiet

# Determine what to deploy: latest release tag, or main if no tags
LATEST_TAG=$(git tag -l 'v*' --sort=-version:refname | head -1)
CURRENT_HEAD=$(git rev-parse HEAD)

if [ -n "$LATEST_TAG" ]; then
    TARGET_REF="$LATEST_TAG"
    TARGET_COMMIT=$(git rev-parse "$LATEST_TAG^{commit}")
    CURRENT_VERSION=$(node -e "console.log(require('./package.json').version)")
    TAG_VERSION="${LATEST_TAG#v}"

    if [ "$CURRENT_HEAD" = "$TARGET_COMMIT" ]; then
        log "Already on latest release $LATEST_TAG ($CURRENT_VERSION)"
        exit 0
    fi

    log "New release detected: v$CURRENT_VERSION -> $LATEST_TAG"
    git checkout "$LATEST_TAG" --quiet
else
    # No tags — fall back to main
    TARGET_COMMIT=$(git rev-parse origin/main)

    if [ "$CURRENT_HEAD" = "$TARGET_COMMIT" ]; then
        log "Already up to date ($CURRENT_HEAD)"
        exit 0
    fi

    log "Update available on main: $CURRENT_HEAD -> $TARGET_COMMIT"
    git checkout origin/main --quiet
fi

# Install dependencies if package-lock changed
if ! git diff --quiet "$CURRENT_HEAD" HEAD -- package-lock.json 2>/dev/null; then
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
if ! git diff --quiet "$CURRENT_HEAD" HEAD -- package-lock.json 2>/dev/null; then
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
