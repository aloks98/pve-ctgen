#!/usr/bin/env bash
#
# pvectgen installer — downloads pre-built binary from GitHub releases
#
# Install:
#   curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install.sh | bash
#
# Install specific version:
#   curl -fsSL .../install.sh | bash -s -- v1.0.0
#
# Install from source instead:
#   curl -fsSL .../install.sh | bash -s -- --source
#
set -euo pipefail

REPO="aloks98/pve-ctgen"
REPO_URL="https://github.com/${REPO}.git"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="pvectgen"
VERSION=""
FROM_SOURCE=false

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --source)  FROM_SOURCE=true; shift ;;
    --dir)     INSTALL_DIR="$2"; shift 2 ;;
    *)         VERSION="$1"; shift ;;
  esac
done

echo
echo -e "${GREEN}pvectgen installer${NC}"
echo

# =============================================================
# Install from source
# =============================================================
if $FROM_SOURCE; then
  CLONE_DIR="${HOME}/.local/share/pvectgen-src"

  command -v go >/dev/null 2>&1 || fail "Go is required for --source. Install from https://go.dev/dl/"
  command -v git >/dev/null 2>&1 || fail "git is required for --source."
  info "Go: $(go version | grep -oE 'go[0-9]+\.[0-9]+')"

  if [[ -d "${CLONE_DIR}/.git" ]]; then
    info "Updating source in ${CLONE_DIR}..."
    cd "${CLONE_DIR}"
    git fetch --all --tags --prune -q
    if [[ -n "${VERSION}" ]]; then
      git checkout "${VERSION}" -q
    else
      git checkout main -q 2>/dev/null || git checkout master -q 2>/dev/null
      git pull -q
    fi
  else
    info "Cloning ${REPO_URL}..."
    git clone -q "${REPO_URL}" "${CLONE_DIR}"
    cd "${CLONE_DIR}"
    [[ -n "${VERSION}" ]] && git checkout "${VERSION}" -q
  fi

  TAG="$(git describe --tags --always 2>/dev/null || echo "dev")"
  ok "Source at ${TAG}"

  info "Building..."
  CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${TAG}" -o "${BINARY_NAME}" ./cmd/pvectgen

  if [[ -w "${INSTALL_DIR}" ]]; then
    mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    info "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  fi
  ok "Installed ${INSTALL_DIR}/${BINARY_NAME} (${TAG})"

  echo
  echo "To update later:  cd ${CLONE_DIR} && git pull && make install"
  echo "Or run:           pvectgen update"
  echo
  exit 0
fi

# =============================================================
# Install pre-built binary from GitHub releases
# =============================================================

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *) fail "Unsupported OS: $OS" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)       GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) fail "Unsupported architecture: $ARCH" ;;
esac

info "Platform: ${OS}/${GOARCH}"

# Determine version
if [[ -z "$VERSION" ]]; then
  info "Fetching latest release..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
  [[ -n "$VERSION" ]] || fail "Could not determine latest version. Pass one: bash install.sh v1.0.0"
fi
info "Version: ${VERSION}"

# Download
TARBALL="${BINARY_NAME}_${VERSION#v}_${OS}_${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${TARBALL}..."
if ! curl -fsSL -o "${TMP_DIR}/${TARBALL}" "$DOWNLOAD_URL"; then
  fail "Download failed. Check the version exists:\n  ${DOWNLOAD_URL}\n\nFor source install: bash install.sh --source"
fi

tar -xzf "${TMP_DIR}/${TARBALL}" -C "$TMP_DIR"
NEW_BINARY="${TMP_DIR}/${BINARY_NAME}"
[[ -f "$NEW_BINARY" ]] || fail "Binary not found in archive"
chmod +x "$NEW_BINARY"

# Install
if [[ -w "${INSTALL_DIR}" ]]; then
  mv "$NEW_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
else
  info "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "$NEW_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
fi

INSTALLED="$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null | awk '{print $2}' || echo "${VERSION}")"
ok "Installed pvectgen ${INSTALLED}"

echo
echo "Next steps:"
echo "  pvectgen manager import       # Import default templates"
echo "  pvectgen manager tui          # Launch TUI"
echo
echo "To update later:  pvectgen update"
