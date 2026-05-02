#!/usr/bin/env bash
set -euo pipefail

GH_REPO="dknathalage/tallyo"
BIN_DIR="${TALLYO_BIN:-$HOME/.local/bin}"
APP_DIR="${TALLYO_APP_DIR:-/Applications}"

for arg in "$@"; do
  case "$arg" in
    -h|--help)
      cat <<EOF
Tallyo desktop installer

Usage: install.sh

Downloads the prebuilt Tallyo desktop app from the latest GitHub release
and installs it for the current platform.

Env:
  TALLYO_BIN      Bin dir for Linux AppImage (default: \$HOME/.local/bin)
  TALLYO_APP_DIR  Install dir for macOS .app (default: /Applications)
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

TOTAL=4
STEP=0

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

banner

step "Checking prerequisites"
need curl
need uname
ok "curl $(curl --version | head -1 | awk '{print $2}')"

pattern="$(detect_asset_pattern)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

step "Locating latest release"
api="https://api.github.com/repos/$GH_REPO/releases/latest"
meta="$tmpdir/release.json"
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
