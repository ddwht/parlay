#!/bin/sh
set -e

REPO="ddwht/parlay"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

OS=$(uname -s)
ARCH=$(uname -m)

case "$ARCH" in
  x86_64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
  Darwin|Linux) ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

VERSION=$(curl -sI "https://github.com/$REPO/releases/latest" | grep -i "^location:" | sed 's/.*tag\///' | tr -d '\r\n')
if [ -z "$VERSION" ]; then
  echo "Failed to determine latest version" >&2
  exit 1
fi

URL="https://github.com/$REPO/releases/download/${VERSION}/parlay_${OS}_${ARCH}.tar.gz"
echo "Downloading parlay ${VERSION} for ${OS}/${ARCH}..."

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -sL "$URL" -o "$TMP/parlay.tar.gz"
tar xzf "$TMP/parlay.tar.gz" -C "$TMP" parlay

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP/parlay" "$INSTALL_DIR/parlay"
else
  sudo mv "$TMP/parlay" "$INSTALL_DIR/parlay"
fi

echo "parlay ${VERSION} installed to ${INSTALL_DIR}/parlay"
