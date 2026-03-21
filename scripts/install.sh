#!/usr/bin/env bash
#
# pvectgen install from source
#
# Install:
#   curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install.sh | bash
#
# Install specific branch/tag:
#   curl -fsSL .../install.sh | bash -s -- --ref v1.0.0
#
# Update (same command — pulls latest and rebuilds):
#   curl -fsSL .../install.sh | bash
#
set -euo pipefail

REPO="https://github.com/aloks98/pve-ctgen.git"
INSTALL_DIR="/usr/local/bin"
CLONE_DIR="${HOME}/.local/share/pvectgen-src"
BINARY_NAME="pvectgen"
REF=""

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
    --ref) REF="$2"; shift 2 ;;
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    *) REF="$1"; shift ;;
  esac
done

# Check Go
command -v go >/dev/null 2>&1 || fail "Go is required. Install from https://go.dev/dl/"
GO_VERSION="$(go version | grep -oE 'go[0-9]+\.[0-9]+')"
info "Go: ${GO_VERSION}"

# Check git
command -v git >/dev/null 2>&1 || fail "git is required."

# Clone or pull
if [[ -d "${CLONE_DIR}/.git" ]]; then
  info "Updating source in ${CLONE_DIR}..."
  cd "${CLONE_DIR}"
  git fetch --all --tags --prune -q
  if [[ -n "${REF}" ]]; then
    git checkout "${REF}" -q
  else
    git checkout main -q 2>/dev/null || git checkout master -q 2>/dev/null
    git pull -q
  fi
else
  info "Cloning ${REPO}..."
  git clone -q "${REPO}" "${CLONE_DIR}"
  cd "${CLONE_DIR}"
  if [[ -n "${REF}" ]]; then
    git checkout "${REF}" -q
  fi
fi

COMMIT="$(git rev-parse --short HEAD)"
TAG="$(git describe --tags --exact-match 2>/dev/null || echo "dev-${COMMIT}")"
ok "Source at ${TAG} (${COMMIT})"

# Build
info "Building..."
CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${TAG}" -o "${BINARY_NAME}" ./cmd/pvectgen
ok "Built ${BINARY_NAME}"

# Install
if [[ -w "${INSTALL_DIR}" ]]; then
  mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
  info "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi
ok "Installed ${INSTALL_DIR}/${BINARY_NAME}"

# Verify
VERSION="$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null || echo "unknown")"
echo
echo -e "${GREEN}pvectgen installed: ${VERSION}${NC}"
echo
echo "Next steps:"
echo "  pvectgen manager import       # Import default templates"
echo "  pvectgen manager tui          # Launch interactive TUI"
echo
echo "To update later, run this script again."
