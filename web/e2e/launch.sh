#!/usr/bin/env bash
# Build the SPA + the real single binary, then run it on a fresh temp data dir.
# The empty DB makes the first request hit the first-run signup path.
# ponytail: temp dir leaks into $TMPDIR on teardown; it's tiny and OS-cleaned.
set -euo pipefail
cd "$(dirname "$0")/.."          # web/
ROOT="$(cd .. && pwd)"           # repo root

npm run build                                   # emits web/build (embedded by go:embed)
cd "$ROOT"
CGO_ENABLED=0 go build -o bin/tallyo-e2e ./cmd/tallyo

# Live Smarts UI tests need a real ANTHROPIC_API_KEY; load it from .env only when
# SMARTS_E2E=1 so the default suite stays key-free and makes no paid calls.
if [ "${SMARTS_E2E:-}" = "1" ] && [ -f .env ]; then
	set -a; . ./.env; set +a
fi

DATA="$(mktemp -d)"
exec ./bin/tallyo-e2e --data-dir "$DATA" --port "${PORT:-8099}"
