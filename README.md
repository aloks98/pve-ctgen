# pvectgen — Proxmox VE Cloud-Init Template Generator

A manager-minion system for creating Proxmox VE virtual machine templates from cloud images. Manage templates, cloud-init configs, and build steps from your workstation; execute builds on any number of Proxmox nodes.

## How it works

```
┌─────────────────────┐          gRPC           ┌─────────────────────┐
│   Manager (your PC) │ ◄─────────────────────► │  Minion (PVE node)  │
│                     │    build events stream   │                     │
│  TUI / CLI          │                          │  downloads images   │
│  YAML file store    │                          │  runs qm commands   │
│  build history      │                          │  streams logs back  │
└─────────────────────┘                          └─────────────────────┘
```

**Single binary, two modes.** The same `pvectgen` binary runs as `manager` on your workstation or as `minion` on Proxmox nodes.

## Installation

### Manager (your workstation)

**Pre-built binary (recommended):**
```bash
curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install.sh | bash
```

**From source:**
```bash
curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install.sh | bash -s -- --source
```

**Or manually:**
```bash
git clone https://github.com/aloks98/pve-ctgen.git
cd pve-ctgen
make install
```

### Minion (Proxmox node)

```bash
curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/install-minion.sh | bash
```

This auto-detects the Proxmox hostname, generates an API key, installs a systemd service, and prints a connection token.

### Updating

```bash
# Self-update (built into the binary)
pvectgen update

# Or via script
curl -fsSL https://raw.githubusercontent.com/aloks98/pve-ctgen/main/scripts/update.sh | bash

# Specific version
pvectgen update v1.2.0
```

## Quick Start

```bash
# 1. Install minion on Proxmox node (see above), copy the connection token

# 2. Connect to your node
pvectgen manager node add --token <token> --display-name "my-node"

# 3. Import default templates and cloud-init configs
pvectgen manager import

# 4. Launch the TUI
pvectgen manager tui
```

## Features

- **Multi-node management** — manage templates across multiple Proxmox nodes from one interface
- **Interactive TUI** — BubbleTea-based terminal UI with live build progress, background builds, scrollable views
- **Full CLI** — every operation available as a CLI command for scripting
- **Cloud-init management** — edit configs in neovim/vim directly from the TUI, with YAML validation
- **Build history** — file-based history of all builds with per-step logs
- **VM launching** — clone templates with hostname, memory, cores, static/DHCP IP, nameserver, search domain
- **Background builds** — press esc during a build to return to menu, build continues in background
- **Self-update** — `pvectgen update` downloads and replaces the binary
- **One-click deploy** — install scripts for both manager and minion
- **Secure** — API key authentication on all gRPC calls
- **File-based storage** — all data in human-readable YAML files, no database

## TUI

```
pvectgen manager tui
```

| Screen | Description |
|--------|-------------|
| **Nodes** | Add/remove Proxmox nodes, health checks |
| **Cloud-Init** | View, add, edit (opens in neovim/vim), delete configs |
| **Templates** | Manage VM template definitions |
| **Steps** | Add, edit, reorder (J/K) build commands |
| **Build** | Select templates → node → override VM IDs → live progress |
| **History** | Browse past builds with scrollable per-step logs |
| **Launch VM** | Pick node → pick template from node → configure → launch |

### TUI Shortcuts

| Key | Context | Action |
|-----|---------|--------|
| `1-7` | Home | Jump to menu item |
| `j/k` | Everywhere | Navigate / scroll |
| `enter` | Lists | Select / view detail |
| `a` | Lists | Add new item |
| `e` | Lists | Edit selected item |
| `d` | Lists | Delete selected item |
| `J/K` | Steps | Reorder steps |
| `space` | Build select | Toggle template |
| `esc` | Sub-views | Go back |
| `esc` | Build progress | Send build to background |
| `q` | Home | Quit |

## CLI Reference

```
pvectgen
├── manager
│   ├── tui                                  # Interactive TUI
│   ├── cloudinit list|add|show|edit|remove   # Cloud-init configs
│   ├── template  list|add|show|edit|remove   # VM templates
│   ├── steps     list|add|edit|remove|reset  # Build steps
│   ├── node      list|add|remove|health      # Proxmox nodes
│   ├── build     run|cancel                  # Trigger builds
│   ├── builds    list|show|logs              # Build history
│   ├── vm        launch|list                 # VM operations
│   └── import    [--force]                   # Import from config files
├── minion
│   ├── serve     [--config path]             # Start gRPC server
│   └── connect   [--config path]             # Show connection token
├── update        [version]                   # Self-update
└── version                                   # Show version
```

## Data Storage

All data is stored as human-readable YAML files:

```
~/.config/pvectgen/
├── config.yaml           # Manager config
├── nodes.yaml            # Node connections
├── templates.yaml        # Template definitions
├── steps.yaml            # Build step commands
├── cloudinit/             # Cloud-init configs (one .yaml per config)
│   ├── ubuntu.yaml
│   ├── debian.yaml
│   └── ...
└── builds/                # Build history (one .yaml per build)
    └── <uuid>.yaml
```

## Configuration

### Manager (`~/.config/pvectgen/config.yaml`)

```yaml
data_dir: "~/.config/pvectgen"
default_node: ""
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

## VM Launch Options

When launching a VM from a template, you can configure:

| Option | Description |
|--------|-------------|
| VM ID | New VM ID on the Proxmox node |
| Name | VM name (used as hostname by cloud-init) |
| Hostname | FQDN — splits into name + search domain (e.g. `web.e412.in` → name `web`, searchdomain `e412.in`) |
| Memory | RAM in MB (default: 2048) |
| Cores | CPU cores (default: 2) |
| IP Config | DHCP or static |
| IP Address | Static IP with CIDR (e.g. `192.168.1.50/24`) |
| Gateway | Gateway IP (e.g. `192.168.1.1`) |
| Nameserver | DNS server (e.g. `8.8.8.8`) |
| Search Domain | DNS search domain (e.g. `e412.in`) |
| Start | Start VM after creation |
| Start at Boot | Auto-start on node boot |

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

## Deployment

### Releases (GitHub Actions)

Create a release through the GitHub UI → CI runs lint + tests → GoReleaser builds binaries for linux/darwin × amd64/arm64 and uploads to the release.

### Deploy minion via SSH

```bash
make deploy NODE=root@192.168.1.100
```

## Development

```bash
make build-local    # Build for current platform
make build          # Cross-compile for Linux amd64
make install        # Build + install to /usr/local/bin
make lint           # go vet
make test           # go test ./...
make proto          # Regenerate protobuf (requires protoc)
```

## Architecture

```
cmd/pvectgen/main.go           Cobra root command + update command
internal/
  shared/                      Shared: models, checksum, download, cloudinit validation, token, fileutil
  manager/
    cli/                       All manager Cobra subcommands
    tui/                       BubbleTea app, views, components, styles, spinners
    store/store.go             File-based YAML store (nodes, templates, steps, builds, cloud-init)
    grpc/client.go             gRPC client to minions
    config/                    Manager YAML config
  minion/
    cli/                       serve + connect commands, Proxmox environment check
    server/                    gRPC server + auth interceptor
    builder/                   Build orchestration with event streaming
    executor/                  Channel-based shell execution
    config/                    Minion YAML config + auto-detection
  update/                      Self-update from GitHub releases
proto/pvectgen/v1/             Protobuf service definition
scripts/
  install.sh                   Install manager (binary or --source)
  install-minion.sh            Install minion on Proxmox nodes
  update.sh                    Update existing installation
```

## License

MIT
