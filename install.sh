#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/dknathalage/tallyo.git"
SRC_DIR="${TALLYO_SRC:-$HOME/.tallyo-src}"
BIN_DIR="${TALLYO_BIN:-$HOME/.local/bin}"

if [ -t 1 ]; then
  BOLD=$'\033[1m'; DIM=$'\033[2m'; RESET=$'\033[0m'
  CYAN=$'\033[36m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; RED=$'\033[31m'; MAGENTA=$'\033[35m'
else
  BOLD=""; DIM=""; RESET=""; CYAN=""; GREEN=""; YELLOW=""; RED=""; MAGENTA=""
fi

TOTAL=4
STEP=0

banner() {
  printf '\n'
  printf '  %s╭─────────────────────────────╮%s\n' "$MAGENTA" "$RESET"
  printf '  %s│%s   %sTallyo%s installer            %s│%s\n' "$MAGENTA" "$RESET" "$BOLD$CYAN" "$RESET" "$MAGENTA" "$RESET"
  printf '  %s╰─────────────────────────────╯%s\n' "$MAGENTA" "$RESET"
  printf '\n'
}

step() {
  STEP=$((STEP + 1))
  printf '  %s[%d/%d]%s %s%s%s\n' "$DIM" "$STEP" "$TOTAL" "$RESET" "$BOLD" "$1" "$RESET"
}

ok() { printf '        %s✓%s %s\n' "$GREEN" "$RESET" "$1"; }
info() { printf '        %s%s%s\n' "$DIM" "$1" "$RESET"; }
warn() { printf '  %s!%s %s\n' "$YELLOW" "$RESET" "$1"; }
fail() { printf '  %s✗%s %s\n' "$RED" "$RESET" "$1" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || fail "'$1' required but not found on PATH"; }

run_quiet() {
  local log
  log=$(mktemp)
  if ! "$@" >"$log" 2>&1; then
    printf '\n%s--- command output ---%s\n' "$DIM" "$RESET" >&2
    cat "$log" >&2
    rm -f "$log"
    fail "step failed: $*"
  fi
  rm -f "$log"
}

banner

step "Checking prerequisites"
need git
need node
need npm
ok "git $(git --version | awk '{print $3}'), node $(node -v), npm $(npm -v)"

mkdir -p "$BIN_DIR"

step "Fetching source"
if [ -d "$SRC_DIR/.git" ]; then
  info "updating $SRC_DIR"
  run_quiet git -C "$SRC_DIR" fetch origin
  run_quiet git -C "$SRC_DIR" reset --hard origin/main
  ok "updated"
else
  info "cloning into $SRC_DIR"
  rm -rf "$SRC_DIR"
  run_quiet git clone --depth 1 "$REPO" "$SRC_DIR"
  ok "cloned"
fi

step "Installing dependencies"
info "this can take a minute"
run_quiet bash -c "cd '$SRC_DIR' && npm install --silent --no-audit --no-fund"
ok "dependencies installed"

step "Building"
run_quiet bash -c "cd '$SRC_DIR' && npm run build --silent"
ok "build complete"

ln -sf "$SRC_DIR/bin/tallyo.js" "$BIN_DIR/tallyo"
chmod +x "$SRC_DIR/bin/tallyo.js"

VERSION=$(node -p "require('$SRC_DIR/package.json').version" 2>/dev/null || echo "?")

printf '\n  %s✓ Installed%s tallyo %sv%s%s\n' "$GREEN$BOLD" "$RESET" "$DIM" "$VERSION" "$RESET"
printf '    %s→%s %s%s/tallyo%s\n' "$DIM" "$RESET" "$CYAN" "$BIN_DIR" "$RESET"

case ":$PATH:" in
  *":$BIN_DIR:"*)
    printf '\n  Run: %s%stallyo%s\n\n' "$BOLD" "$CYAN" "$RESET"
    ;;
  *)
    printf '\n'
    warn "$BIN_DIR is not on your PATH"
    printf '    Add to your shell rc:\n'
    printf '      %sexport PATH="%s:$PATH"%s\n\n' "$DIM" "$BIN_DIR" "$RESET"
    ;;
esac
