#!/usr/bin/env bash
#
# pvectgen updater — updates existing installation from GitHub releases
#
# Update to latest:
#   curl --connect-timeout 15 --max-time 600 -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/update.sh | bash
#
# Update to specific version:
#   curl --connect-timeout 15 --max-time 600 -fsSL .../update.sh | bash -s -- v1.2.0
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

# Stage on the destination filesystem, then rename over the executable.
atomic_install() {
  local source_file="$1" staged_file
  if [[ -w "$INSTALL_DIR" ]]; then
    staged_file="$(mktemp "${INSTALL_DIR}/.pvectgen.XXXXXX")"
    if ! install -m 0755 "$source_file" "$staged_file" || ! mv -f "$staged_file" "${INSTALL_DIR}/${BINARY_NAME}"; then
      rm -f "$staged_file"; return 1
    fi
  else
    staged_file="$(sudo mktemp "${INSTALL_DIR}/.pvectgen.XXXXXX")"
    if ! sudo install -m 0755 "$source_file" "$staged_file" || ! sudo mv -f "$staged_file" "${INSTALL_DIR}/${BINARY_NAME}"; then
      sudo rm -f "$staged_file"; return 1
    fi
  fi
}

verify_archive() {
  local expected actual
  curl --connect-timeout 15 --max-time 120 -fsSL -o "${TMP_DIR}/checksums.txt" "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
  expected="$(awk -v name="$TARBALL" '$2 == name {print $1}' "${TMP_DIR}/checksums.txt")"
  [[ "$expected" =~ ^[a-fA-F0-9]{64}$ ]] || fail "Missing or ambiguous SHA-256 checksum for $TARBALL"
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${TMP_DIR}/${TARBALL}" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "${TMP_DIR}/${TARBALL}" | awk '{print $1}')"
  fi
  [[ "$actual" == "$expected" ]] || fail "Archive checksum mismatch"
}

echo
echo -e "${GREEN}pvectgen updater${NC}"
echo

# Find current binary
if ! command -v "${BINARY_NAME}" >/dev/null 2>&1; then
  fail "pvectgen is not installed. Use install.sh instead:\n  curl --connect-timeout 15 --max-time 600 -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | bash"
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
  VERSION="$(curl --connect-timeout 15 --max-time 600 -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
  [[ -n "$VERSION" ]] || fail "Could not determine latest version."
fi

if [[ "$CURRENT" == "$VERSION" || "$CURRENT" == "${VERSION#v}" ]]; then
  ok "Already up to date (${CURRENT})"
  exit 0
fi

info "Updating: ${CURRENT} -> ${VERSION}"

# Download
[[ "$VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "Invalid release version"
VERSION="v${VERSION#v}"
TARBALL="${BINARY_NAME}_${VERSION#v}_${OS}_${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${TARBALL}..."
curl --connect-timeout 15 --max-time 600 -fsSL -o "${TMP_DIR}/${TARBALL}" "$DOWNLOAD_URL" || fail "Download failed."

verify_archive
# Extract only the binary to stdout; archive paths are never created.
tar -xOzf "${TMP_DIR}/${TARBALL}" pvectgen > "${TMP_DIR}/${BINARY_NAME}"
[[ -s "${TMP_DIR}/${BINARY_NAME}" ]] || fail "Empty release binary"
NEW_BINARY="${TMP_DIR}/${BINARY_NAME}"
[[ -f "$NEW_BINARY" ]] || fail "Binary not found in archive"
chmod +x "$NEW_BINARY"

# Replace
atomic_install "$NEW_BINARY"

INSTALLED="$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null | awk '{print $2}' || echo "${VERSION}")"
ok "Updated pvectgen ${CURRENT} -> ${INSTALLED}"
