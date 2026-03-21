#!/usr/bin/env bash
#
# pvectgen updater — updates existing installation from GitHub releases
#
# Update to latest:
#   curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/update.sh | bash
#
# Update to specific version:
#   curl -fsSL .../update.sh | bash -s -- v1.2.0
#
# Same as: pvectgen update
#
set -euo pipefail

REPO="aloks98/pve-ctgen"
BINARY_NAME="pvectgen"
INSTALL_DIR="/usr/local/bin"
VERSION="${1:-}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

echo
echo -e "${GREEN}pvectgen updater${NC}"
echo

# Find current binary
if ! command -v "${BINARY_NAME}" >/dev/null 2>&1; then
  fail "pvectgen is not installed. Use install.sh instead:\n  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | bash"
fi

CURRENT="$("${BINARY_NAME}" version 2>/dev/null | awk '{print $2}' || echo "unknown")"
BINARY_PATH="$(command -v "${BINARY_NAME}")"
INSTALL_DIR="$(dirname "$BINARY_PATH")"
info "Current: ${CURRENT} (${BINARY_PATH})"

# Detect platform
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) fail "Unsupported architecture: $ARCH" ;;
esac

# Determine target version
if [[ -z "$VERSION" ]]; then
  info "Checking latest release..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
  [[ -n "$VERSION" ]] || fail "Could not determine latest version."
fi

if [[ "$CURRENT" == "$VERSION" || "$CURRENT" == "${VERSION#v}" ]]; then
  ok "Already up to date (${CURRENT})"
  exit 0
fi

info "Updating: ${CURRENT} -> ${VERSION}"

# Download
TARBALL="${BINARY_NAME}_${VERSION#v}_${OS}_${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${TARBALL}..."
curl -fsSL -o "${TMP_DIR}/${TARBALL}" "$DOWNLOAD_URL" || fail "Download failed."

tar -xzf "${TMP_DIR}/${TARBALL}" -C "$TMP_DIR"
NEW_BINARY="${TMP_DIR}/${BINARY_NAME}"
[[ -f "$NEW_BINARY" ]] || fail "Binary not found in archive"
chmod +x "$NEW_BINARY"

# Replace
if [[ -w "${INSTALL_DIR}" ]]; then
  mv "$NEW_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
else
  info "Replacing ${BINARY_PATH} (requires sudo)..."
  sudo mv "$NEW_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
fi

INSTALLED="$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null | awk '{print $2}' || echo "${VERSION}")"
ok "Updated pvectgen ${CURRENT} -> ${INSTALLED}"
