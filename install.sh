#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/dknathalage/tallyo.git"
SRC_DIR="${TALLYO_SRC:-$HOME/.tallyo-src}"
BIN_DIR="${TALLYO_BIN:-$HOME/.local/bin}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: '$1' required but not found" >&2
    exit 1
  }
}

need git
need node
need npm

mkdir -p "$BIN_DIR"

if [ -d "$SRC_DIR/.git" ]; then
  echo "==> Updating $SRC_DIR"
  git -C "$SRC_DIR" fetch --quiet origin
  git -C "$SRC_DIR" reset --quiet --hard origin/main
else
  echo "==> Cloning into $SRC_DIR"
  rm -rf "$SRC_DIR"
  git clone --quiet --depth 1 "$REPO" "$SRC_DIR"
fi

echo "==> Installing dependencies"
(cd "$SRC_DIR" && npm install --silent --no-audit --no-fund)

echo "==> Building"
(cd "$SRC_DIR" && npm run build --silent)

ln -sf "$SRC_DIR/bin/tallyo.js" "$BIN_DIR/tallyo"
chmod +x "$SRC_DIR/bin/tallyo.js"

echo
echo "==> Installed: $BIN_DIR/tallyo -> $SRC_DIR/bin/tallyo.js"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    echo
    echo "Add this to your shell rc:"
    echo "  export PATH=\"$BIN_DIR:\$PATH\""
    ;;
esac

echo
echo "Run: tallyo"
