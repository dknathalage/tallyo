#!/usr/bin/env bash
set -euo pipefail

GH_REPO="dknathalage/tallyo"
BIN_DIR="${TALLYO_BIN:-$HOME/.local/bin}"

for arg in "$@"; do
  case "$arg" in
    -h|--help)
      cat <<EOF
Tallyo installer

Usage: install.sh

Downloads the prebuilt 'tallyo' server binary from the latest GitHub release
and installs it for the current platform.

Env:
  TALLYO_BIN  Install dir for the binary (default: \$HOME/.local/bin)
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

# Maps the host OS/arch to the goreleaser archive name produced by
# .goreleaser.yaml: tallyo_<version>_<os>_<arch>.<ext>
detect_asset_pattern() {
  local os arch goos goarch ext
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Darwin) goos="darwin"; ext='tar\.gz' ;;
    Linux)  goos="linux";  ext='tar\.gz' ;;
    MINGW*|MSYS*|CYGWIN*) goos="windows"; ext='zip' ;;
    *) fail "unsupported OS: $os" ;;
  esac
  case "$arch" in
    x86_64|amd64)  goarch="amd64" ;;
    arm64|aarch64) goarch="arm64" ;;
    *) fail "unsupported arch: $arch" ;;
  esac
  echo "tallyo_.*_${goos}_${goarch}\.${ext}\$"
}

banner

step "Checking prerequisites"
need curl
need uname
need tar
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
printf '        '
if ! curl -fL --progress-bar -o "$tmpdir/$asset_name" "$asset_url"; then
  fail "download failed"
fi
ok "downloaded $(du -h "$tmpdir/$asset_name" | awk '{print $1}')"

step "Installing"
extract="$tmpdir/extract"
mkdir -p "$extract"
case "$asset_name" in
  *.tar.gz) run_quiet tar -xzf "$tmpdir/$asset_name" -C "$extract" ;;
  *.zip)    need unzip; run_quiet unzip -q -o "$tmpdir/$asset_name" -d "$extract" ;;
  *)        fail "unhandled asset type: $asset_name" ;;
esac

bin_src="$(find "$extract" -maxdepth 2 -name 'tallyo' -type f -print -quit)"
[ -z "$bin_src" ] && bin_src="$(find "$extract" -maxdepth 2 -name 'tallyo.exe' -type f -print -quit)"
[ -n "$bin_src" ] || fail "no 'tallyo' binary found in archive"

mkdir -p "$BIN_DIR"
install -m 0755 "$bin_src" "$BIN_DIR/$(basename "$bin_src")"
ok "installed to $BIN_DIR/$(basename "$bin_src")"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) warn "$BIN_DIR is not on your PATH — add it to run 'tallyo' directly" ;;
esac

printf '\n  Run: %stallyo --port 8080%s\n\n' "$BOLD$CYAN" "$RESET"
