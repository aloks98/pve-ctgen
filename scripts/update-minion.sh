#!/usr/bin/env bash
#
# pvectgen minion updater
#
# Update to latest:
#   curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/update-minion.sh | bash
#
# Update to specific version:
#   curl -fsSL .../update-minion.sh | bash -s -- v1.2.0
#
set -euo pipefail

REPO="aloks98/pve-ctgen"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="pvectgen"
SERVICE_NAME="pvectgen-minion"
VERSION="${1:-}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

echo
echo -e "${GREEN}pvectgen minion updater${NC}"
echo

# --- Pre-flight ---

[[ "$(uname -s)" == "Linux" ]] || fail "This script only runs on Linux."
[[ "$(id -u)" -eq 0 ]] || fail "Run as root: sudo bash update-minion.sh"
command -v pveversion >/dev/null 2>&1 || fail "Proxmox VE not detected."

# --- Current version ---

CURRENT="none"
if command -v "${BINARY_NAME}" >/dev/null 2>&1; then
  CURRENT="$("${BINARY_NAME}" version 2>/dev/null | awk '{print $2}' || echo "unknown")"
fi
info "Current: ${CURRENT}"
info "Proxmox: $(pveversion)"

# --- Detect arch ---

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *) fail "Unsupported architecture: $ARCH" ;;
esac

# --- Determine version ---

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

# --- Download ---

TARBALL="${BINARY_NAME}_${VERSION#v}_linux_${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${TARBALL}..."
curl -fsSL -o "${TMP_DIR}/${TARBALL}" "$DOWNLOAD_URL" || fail "Download failed."

tar -xzf "${TMP_DIR}/${TARBALL}" -C "$TMP_DIR"
NEW_BINARY="${TMP_DIR}/${BINARY_NAME}"
[[ -f "$NEW_BINARY" ]] || fail "Binary not found in archive"
chmod +x "$NEW_BINARY"

# --- Stop, replace, start ---

info "Stopping ${SERVICE_NAME}..."
systemctl stop "$SERVICE_NAME" 2>/dev/null || true

mv "$NEW_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"

systemctl start "$SERVICE_NAME"
ok "Service restarted"

# --- Verify ---

INSTALLED="$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null | awk '{print $2}' || echo "${VERSION}")"
ok "Updated pvectgen ${CURRENT} -> ${INSTALLED}"
