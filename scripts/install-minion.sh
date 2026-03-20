#!/usr/bin/env bash
#
# pvectgen minion installer
#
# One-line install:
#   curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install-minion.sh | bash
#
# Or with a specific version:
#   curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install-minion.sh | bash -s -- v1.0.0
#
set -euo pipefail

REPO="aloks98/pve-ctgen"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/pvectgen"
SERVICE_NAME="pvectgen-minion"
BINARY_NAME="pvectgen"

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

# --- Pre-flight checks ---

[[ "$(uname -s)" == "Linux" ]] || fail "This installer only runs on Linux."
[[ "$(id -u)" -eq 0 ]] || fail "Run as root: sudo bash install-minion.sh"

command -v pveversion >/dev/null 2>&1 || fail "Proxmox VE not detected (pveversion not found)."
command -v qm >/dev/null 2>&1 || fail "'qm' not found. Is Proxmox VE fully installed?"
[[ -d /etc/pve ]] || fail "/etc/pve not found. Is this a Proxmox node?"

ok "Proxmox VE detected: $(pveversion)"

# --- Detect architecture ---

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *)       fail "Unsupported architecture: $ARCH" ;;
esac

info "Architecture: ${ARCH} (${GOARCH})"

# --- Determine version ---

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  info "Fetching latest release..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
  [[ -n "$VERSION" ]] || fail "Could not determine latest version. Pass a version: bash install-minion.sh v1.0.0"
fi

info "Version: ${VERSION}"

# --- Download ---

TARBALL="${BINARY_NAME}_${VERSION#v}_linux_${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${DOWNLOAD_URL}..."
curl -fsSL -o "${TMP_DIR}/${TARBALL}" "$DOWNLOAD_URL" || fail "Download failed. Check the version and URL."
ok "Downloaded."

# --- Install binary ---

info "Extracting..."
tar -xzf "${TMP_DIR}/${TARBALL}" -C "$TMP_DIR"

# Stop service if running (ignore errors for fresh installs)
systemctl stop "$SERVICE_NAME" 2>/dev/null || true

install -m 0755 "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
ok "Installed ${INSTALL_DIR}/${BINARY_NAME}"

# --- Config ---

mkdir -p "$CONFIG_DIR"
if [[ ! -f "${CONFIG_DIR}/minion.yaml" ]]; then
  cat > "${CONFIG_DIR}/minion.yaml" <<'YAML'
# pvectgen Minion configuration
# Node name and API key are auto-detected/generated on first run

listen_address: "0.0.0.0:50051"
node_name: ""
api_key: ""
iso_path: "/var/lib/vz/template/iso"
snippets_path: "/var/lib/vz/snippets"
work_dir: "/tmp/pvectgen"
YAML
  ok "Created ${CONFIG_DIR}/minion.yaml"
else
  warn "Config already exists at ${CONFIG_DIR}/minion.yaml — skipping."
fi

# --- Systemd service ---

cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=pvectgen Minion - Proxmox VE Template Build Agent
After=network.target pve-cluster.service

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} minion serve --config ${CONFIG_DIR}/minion.yaml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
User=root

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl start "$SERVICE_NAME"
ok "Service ${SERVICE_NAME} enabled and started."

# --- Show connection info ---

echo
"${INSTALL_DIR}/${BINARY_NAME}" minion connect --config "${CONFIG_DIR}/minion.yaml"

echo
echo -e "${GREEN}Installation complete!${NC}"
echo
echo "On your workstation, run:"
echo "  pvectgen manager node add --token <token-from-above>"
echo
echo "Useful commands:"
echo "  systemctl status ${SERVICE_NAME}     # Check service status"
echo "  journalctl -u ${SERVICE_NAME} -f     # Follow logs"
echo "  pvectgen minion connect              # Show connection token again"
