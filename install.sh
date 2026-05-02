#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/dknathalage/tallyo.git"
GH_REPO="dknathalage/tallyo"
SRC_DIR="${TALLYO_SRC:-$HOME/.tallyo-src}"
BIN_DIR="${TALLYO_BIN:-$HOME/.local/bin}"
APP_DIR="${TALLYO_APP_DIR:-/Applications}"

MODE="cli"
for arg in "$@"; do
  case "$arg" in
    --desktop) MODE="desktop" ;;
    --cli)     MODE="cli" ;;
    -h|--help)
      cat <<EOF
Tallyo installer

Usage: install.sh [--cli|--desktop]

  --cli       Install the Node CLI (default). Builds from source.
  --desktop   Download the prebuilt desktop app from the latest GitHub release.

Env:
  TALLYO_SRC      Source dir for CLI install (default: \$HOME/.tallyo-src)
  TALLYO_BIN      Bin dir for CLI symlink   (default: \$HOME/.local/bin)
  TALLYO_APP_DIR  Install dir for desktop app on macOS (default: /Applications)
EOF
      exit 0
      ;;
  esac
done

if [ -t 1 ]; then
  BOLD=$'\033[1m'; DIM=$'\033[2m'; RESET=$'\033[0m'
  CYAN=$'\033[36m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; RED=$'\033[31m'; MAGENTA=$'\033[35m'
else
  BOLD=""; DIM=""; RESET=""; CYAN=""; GREEN=""; YELLOW=""; RED=""; MAGENTA=""
fi

STEP=0
TOTAL=5

banner() {
  printf '\n'
  printf '  %s╭──────────────────────────╮%s\n' "$MAGENTA" "$RESET"
  printf '  %s│%s   %sTallyo%s installer       %s│%s\n' "$MAGENTA" "$RESET" "$BOLD$CYAN" "$RESET" "$MAGENTA" "$RESET"
  printf '  %s╰──────────────────────────╯%s\n' "$MAGENTA" "$RESET"
  printf '\n'
}

step() {
  STEP=$((STEP + 1))
  printf '  %s[%d/%d]%s %s%s%s\n' "$DIM" "$STEP" "$TOTAL" "$RESET" "$BOLD" "$1" "$RESET"
}

ok()   { printf '        %s✓%s %s\n' "$GREEN" "$RESET" "$1"; }
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

install_cli() {
  TOTAL=5
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

  SHA=$(git -C "$SRC_DIR" rev-parse --short HEAD)
  BUILT_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  printf '{"commit":"%s","builtAt":"%s"}\n' "$SHA" "$BUILT_AT" > "$SRC_DIR/bin/.tallyo-build.json"

  ln -sf "$SRC_DIR/bin/tallyo.js" "$BIN_DIR/tallyo"
  chmod +x "$SRC_DIR/bin/tallyo.js"

  step "Running migrations"
  run_quiet node "$SRC_DIR/bin/tallyo.js" --migrate
  ok "database up to date"

  VERSION=$(node -p "require('$SRC_DIR/package.json').version" 2>/dev/null || echo "?")

  printf '\n  %s✓ Installed%s tallyo %sv%s (%s)%s\n' "$GREEN$BOLD" "$RESET" "$DIM" "$VERSION" "$SHA" "$RESET"
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
}

detect_asset_pattern() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Darwin)
      case "$arch" in
        arm64)  echo 'Tallyo-.*-arm64\.dmg$' ;;
        x86_64) echo 'Tallyo-.*-x64\.dmg$|Tallyo-.*\.dmg$' ;;
        *)      fail "unsupported macOS arch: $arch" ;;
      esac
      ;;
    Linux)  echo 'Tallyo-.*\.AppImage$' ;;
    MINGW*|MSYS*|CYGWIN*) echo 'Tallyo.*Setup.*\.exe$' ;;
    *) fail "unsupported OS: $os" ;;
  esac
}

install_desktop() {
  TOTAL=4
  step "Checking prerequisites"
  need curl
  need uname
  ok "curl $(curl --version | head -1 | awk '{print $2}')"

  local pattern asset_url asset_name tmpdir
  pattern="$(detect_asset_pattern)"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  step "Locating latest release"
  local api="https://api.github.com/repos/$GH_REPO/releases/latest"
  local meta="$tmpdir/release.json"
  run_quiet curl -fsSL -H 'Accept: application/vnd.github+json' "$api" -o "$meta"

  asset_name="$(grep -oE '"name":[[:space:]]*"[^"]+"' "$meta" | sed 's/.*"\([^"]*\)"$/\1/' | grep -E "$pattern" | head -1 || true)"
  [ -n "$asset_name" ] || fail "no matching asset for this platform in latest release"
  asset_url="$(grep -oE '"browser_download_url":[[:space:]]*"[^"]+"' "$meta" | sed 's/.*"\([^"]*\)"$/\1/' | grep -F "$asset_name" | head -1)"
  [ -n "$asset_url" ] || fail "could not resolve download url for $asset_name"
  ok "$asset_name"

  step "Downloading"
  run_quiet curl -fsSL -o "$tmpdir/$asset_name" "$asset_url"
  ok "downloaded $(du -h "$tmpdir/$asset_name" | awk '{print $1}')"

  step "Installing"
  case "$asset_name" in
    *.dmg)
      need hdiutil
      local mnt
      mnt="$(hdiutil attach -nobrowse -quiet "$tmpdir/$asset_name" | tail -1 | awk '{print $3}')"
      [ -n "$mnt" ] || fail "failed to mount dmg"
      mkdir -p "$APP_DIR"
      rm -rf "$APP_DIR/Tallyo.app"
      run_quiet cp -R "$mnt/Tallyo.app" "$APP_DIR/"
      hdiutil detach -quiet "$mnt" || true
      ok "installed to $APP_DIR/Tallyo.app"
      printf '\n  Run: %sopen -a Tallyo%s\n\n' "$BOLD$CYAN" "$RESET"
      ;;
    *.AppImage)
      mkdir -p "$BIN_DIR"
      install -m 0755 "$tmpdir/$asset_name" "$BIN_DIR/tallyo"
      ok "installed to $BIN_DIR/tallyo"
      printf '\n  Run: %s%s/tallyo%s\n\n' "$BOLD$CYAN" "$BIN_DIR" "$RESET"
      ;;
    *.exe)
      info "launch the installer manually:"
      printf '    %s%s%s\n\n' "$CYAN" "$tmpdir/$asset_name" "$RESET"
      ;;
    *)
      fail "unhandled asset type: $asset_name"
      ;;
  esac
}

banner

case "$MODE" in
  cli)     install_cli ;;
  desktop) install_desktop ;;
esac
