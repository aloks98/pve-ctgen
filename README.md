# pvectgen — Proxmox VE Cloud-Init Template Generator

A manager-minion system for creating Proxmox VE virtual machine templates from cloud images. Manage templates, cloud-init configs, and build steps from your workstation; execute builds on any number of Proxmox nodes.

## How it works

```
┌─────────────────────┐          gRPC           ┌─────────────────────┐
│   Manager (your PC) │ ◄─────────────────────► │  Minion (PVE node)  │
│                     │    build events stream   │                     │
│  TUI / CLI          │                          │  downloads images   │
│  SQLite store       │                          │  runs qm commands   │
│  build history      │                          │  streams logs back  │
└─────────────────────┘                          └─────────────────────┘
```

**Single binary, two modes.** The same `pvectgen` binary runs as `manager` on your workstation or as `minion` on Proxmox nodes.

## Quick Start

### 1. Install the Minion on your Proxmox node

```bash
curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install-minion.sh | bash
```

This auto-detects the Proxmox hostname, generates an API key, installs a systemd service, and prints a connection token.

### 2. Install the Manager on your workstation

**macOS (Homebrew):**
```bash
# From GitHub releases
curl -fsSL https://github.com/aloks98/pve-ctgen/releases/latest/download/pvectgen_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/').tar.gz | tar xz
sudo mv pvectgen /usr/local/bin/
```

**From source:**
```bash
git clone https://github.com/aloks98/pve-ctgen.git
cd pve-ctgen
make build-local
```

### 3. Connect to your node

```bash
pvectgen manager node add --token <token-from-step-1> --display-name "my-node"
pvectgen manager node health
```

### 4. Import default templates and launch the TUI

```bash
pvectgen manager import
pvectgen manager tui
```

## Features

- **Multi-node management** — manage templates across multiple Proxmox nodes from one interface
- **Interactive TUI** — BubbleTea-based terminal UI with live build progress streaming
- **Full CLI** — every operation available as a CLI command for scripting
- **Cloud-init management** — store, edit, and validate cloud-init configs with YAML validation
- **Build history** — SQLite-backed history of all builds with per-step logs
- **VM launching** — clone templates with configurable memory, cores, IP (DHCP or static)
- **One-click deploy** — install script + systemd service for Proxmox nodes
- **Secure** — API key authentication on all gRPC calls

## TUI

```
pvectgen manager tui
```

| Screen | Description |
|--------|-------------|
| **Nodes** | Add/remove Proxmox nodes, health checks |
| **Cloud-Init Store** | Add, edit (inline YAML editor), delete configs |
| **Template Store** | Manage VM template definitions |
| **Build Steps** | Add, edit, reorder (J/K) build commands |
| **New Build** | Select templates + node, live progress view |
| **Build History** | Browse past builds, view per-step logs |
| **Launch VM** | Pick node → pick template → configure → launch |

## CLI Reference

```
pvectgen manager cloudinit list|add|show|edit|remove
pvectgen manager template  list|add|show|edit|remove
pvectgen manager steps     list|add|edit|remove|reset
pvectgen manager node      list|add|remove|health
pvectgen manager build     run --template name --node name [--all]
pvectgen manager builds    list|show|logs
pvectgen manager vm        launch|list
pvectgen manager import    [--images path] [--steps path] [--cloudinit-dir path]
pvectgen manager tui

pvectgen minion serve      [--config /etc/pvectgen/minion.yaml]
pvectgen minion connect    [--config /etc/pvectgen/minion.yaml]
```

## Deployment

### Automated (GitHub Releases)

Every tagged release builds binaries for Linux and macOS (amd64 + arm64) via GoReleaser.

**Install minion on Proxmox:**
```bash
curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install-minion.sh | bash
# Or with a specific version:
curl -fsSL .../install-minion.sh | bash -s -- v1.0.0
```

**Deploy via SSH (from your workstation):**
```bash
make deploy NODE=root@192.168.1.100
```

### Creating a release

```bash
make release VERSION=v1.0.0
# Tags, pushes → GitHub Actions builds + publishes
```

### Local snapshot build (all platforms)

```bash
make snapshot
ls dist/
```

## Configuration

### Manager (`~/.config/pvectgen/config.yaml`)

```yaml
db_path: "~/.config/pvectgen/pvectgen.db"
default_node: "my-node"
```

### Minion (`/etc/pvectgen/minion.yaml`)

```yaml
listen_address: "0.0.0.0:50051"
node_name: ""        # Auto-detected from Proxmox hostname
api_key: ""          # Auto-generated on first run
iso_path: "/var/lib/vz/template/iso"
snippets_path: "/var/lib/vz/snippets"
work_dir: "/tmp/pvectgen"
```

## Template Variables

Build step commands support these placeholders:

| Variable | Source | Example |
|----------|--------|---------|
| `{{.ID}}` | Template VM ID | `8201` |
| `{{.Name}}` | Template name | `ubuntu2404` |
| `{{.Tags}}` | Comma-separated tags | `ubuntu,cloudinit` |
| `{{.Vendor}}` | Cloud-init filename | `ubuntu.yaml` |
| `{{.FilePath}}` | Working image path | `/tmp/pvectgen/base.qcow2` |

## Supported OS Images (defaults)

| VM ID | Name | Distribution |
|-------|------|-------------|
| 8201 | ubuntu2404 | Ubuntu 24.04 LTS |
| 8202 | debian13 | Debian 13 |
| 8203 | debian12 | Debian 12 |
| 8204 | alma10 | AlmaLinux 10 |
| 8205 | alma9 | AlmaLinux 9 |
| 8206 | fedora43 | Fedora 43 |
| 8207 | rocky10 | Rocky Linux 10 |
| 8208 | rocky9 | Rocky Linux 9 |

Add more via `pvectgen manager template add` or by editing `config/os_list.json` and running `pvectgen manager import`.

## Development

```bash
make build-local    # Build for current platform
make build          # Cross-compile for Linux amd64
make lint           # go vet
make test           # go test ./...
make proto          # Regenerate protobuf (requires protoc)
make snapshot       # GoReleaser local build (all platforms)
```

## Architecture

```
cmd/pvectgen/main.go           Cobra root command
internal/
  shared/                      Shared code (models, checksum, download, validation, token)
  manager/
    cli/                       All manager Cobra subcommands
    tui/                       BubbleTea app, views, components, styles
    store/                     SQLite CRUD + migrations
    grpc/client.go             gRPC client to minions
    config/                    Manager YAML config
  minion/
    cli/                       serve + connect commands, Proxmox checks
    server/                    gRPC server + auth interceptor
    builder/                   Build orchestration
    executor/                  Channel-based shell execution
    config/                    Minion YAML config + auto-detection
proto/pvectgen/v1/             Protobuf service definition
```

## License

MIT
