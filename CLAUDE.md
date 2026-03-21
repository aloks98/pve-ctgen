# CLAUDE.md - Project Context for AI Assistants

This file provides context for Claude and other AI assistants when working on the pve-ctgen codebase.

## Project Overview

**pve-ctgen** (Proxmox VE Cloud-Init Template Generator) is a Go-based manager-minion system for creating Proxmox VE virtual machine templates from cloud images. A single binary provides two modes:

- **Manager** (`pvectgen manager ...`): CLI/TUI on user's workstation — manages templates, cloud-init configs, build steps, nodes, triggers builds, launches VMs
- **Minion** (`pvectgen minion serve`): Headless agent on each Proxmox node — receives commands via gRPC, executes builds, streams logs back

## Tech Stack

| Component | Choice |
|-----------|--------|
| Language | Go 1.24+ |
| CLI framework | Cobra |
| TUI framework | BubbleTea + Lipgloss + Bubbles |
| Manager-Minion comms | gRPC (server streaming for builds) |
| Manager storage | File-based YAML (no database) |
| Config format | YAML via `gopkg.in/yaml.v3` |
| Auth | API key in gRPC metadata |
| Proxmox interaction | `qm`, `pvesh` shell commands |
| CI/CD | GitHub Actions + GoReleaser |

## Project Structure

```
pve-ctgen/
├── cmd/pvectgen/main.go              # Single binary entry point (Cobra root + update cmd)
├── internal/
│   ├── shared/
│   │   ├── models/models.go          # All data models (no DB IDs, plain YAML-friendly structs)
│   │   ├── checksum/checksum.go      # Checksum parsing and verification
│   │   ├── cloudinit/validate.go     # YAML validation for cloud-init configs
│   │   ├── download/download.go      # HTTP download with progress callback
│   │   ├── token/token.go            # Connection token encode/decode
│   │   └── fileutil/                 # File utilities + editor detection (nvim->vim->vi)
│   ├── manager/
│   │   ├── cli/                      # All Cobra commands
│   │   ├── tui/
│   │   │   ├── app.go               # BubbleTea root model, view routing, build orchestration
│   │   │   ├── views/               # home, nodes, cloudinit, templates, steps, builds, buildselect, buildprogress, vmlaunch
│   │   │   ├── components/           # table, form, logviewer, statusbar, spinner
│   │   │   └── styles/styles.go     # Lipgloss theme (btop-inspired, ASCII status tags)
│   │   ├── store/store.go           # File-based YAML store (all CRUD in one file)
│   │   ├── grpc/client.go           # gRPC client to Minion
│   │   └── config/config.go         # Manager YAML config (data_dir, default_node)
│   ├── minion/
│   │   ├── cli/                      # serve, connect + Proxmox environment check
│   │   ├── server/server.go         # gRPC server + API key interceptor
│   │   ├── executor/executor.go     # Channel-based shell command execution
│   │   ├── builder/builder.go       # Build orchestration with event streaming
│   │   └── config/config.go         # Minion YAML config + auto node name + API key gen
│   └── update/update.go             # Self-update from GitHub releases
├── proto/pvectgen/v1/               # Protobuf service definition + generated code
├── configs/                         # Example configs + systemd unit
├── scripts/
│   ├── install.sh                   # Install manager (binary or --source)
│   ├── install-minion.sh            # Install minion on Proxmox nodes
│   └── update.sh                    # Update existing installation
├── config/                          # Seed data (os_list.json, steps.json)
├── cloudinit/                       # Seed cloud-init configs (no hostname field)
├── .github/workflows/               # CI + release pipelines
├── .goreleaser.yml                  # Multi-platform release config
└── Makefile
```

## Data Storage

File-based YAML, no database:

```
~/.config/pvectgen/
├── config.yaml           # Manager config
├── nodes.yaml            # [{name, display_name, address, api_key}]
├── templates.yaml        # [{vm_id, name, url, checksum_url, tags, cloudinit}]
├── steps.yaml            # [{name, command}]
├── cloudinit/             # One .yaml file per cloud-init config
└── builds/                # One .yaml per build with inline step results
```

Templates reference cloud-init configs by filename (`cloudinit: ubuntu.yaml`), not by ID.

## CLI Command Tree

```
pvectgen
├── manager
│   ├── tui                          # Interactive TUI
│   ├── cloudinit list|add|show|edit|remove
│   ├── template  list|add|show|edit|remove
│   ├── steps     list|add|edit|remove|reset
│   ├── node      list|add|remove|health
│   ├── build     run|cancel
│   ├── builds    list|show|logs
│   ├── vm        launch|list
│   └── import    [--force]
├── minion
│   ├── serve [--config path]
│   └── connect
├── update [version]
└── version
```

## gRPC Service

```protobuf
service MinionService {
  rpc Build(BuildRequest) returns (stream BuildEvent);
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc LaunchVM(LaunchVMRequest) returns (LaunchVMResponse);
  rpc ListTemplates(ListTemplatesRequest) returns (ListTemplatesResponse);
}
```

LaunchVM supports: template_id, new_vm_id, name, hostname, memory, cores, ip_config, nameserver, search_domain, start, start_at_boot.

Auth: API key in `x-api-key` gRPC metadata, validated by unary+stream interceptor.

## Key Workflows

### Minion Onboarding
1. Install on Proxmox: `curl -fsSL .../install-minion.sh | bash`
2. Auto-detects hostname from Proxmox, generates API key, outputs connection token
3. On workstation: `pvectgen manager node add --token <token> --display-name "prod"`

### Build Flow (TUI)
1. Home → New Build → select templates (space/a) → enter → select node → enter
2. Override VM IDs per template (pre-filled with defaults)
3. App connects to minion via gRPC, streams BuildEvents back
4. Live split-pane progress: step tree (left, collapses per-build) + logs (right, clears between builds)
5. Press esc to send build to background — indicator shows on home screen
6. Results persisted to `builds/<uuid>.yaml`

### VM Launch (TUI)
1. Select node → fetches templates from minion via `pvesh` API
2. Select template → configure VM options (hostname, memory, cores, IP, DNS, etc.)
3. Hostname FQDN (e.g. `web.e412.in`) auto-splits into VM name (`web`) + search domain (`e412.in`)
4. Static IP shows extra fields: address, gateway, nameserver, search domain
5. Launches via gRPC LaunchVM

### Cloud-Init Editing
- TUI: press `e` → opens neovim (or vim/vi) via `tea.ExecProcess` → validates YAML on save
- Editor fallback chain: `$EDITOR` → `nvim` → `vim` → `vi`
- Cloud-init configs have no `hostname:` field — Proxmox sets hostname from VM name

## Build Commands

```bash
make build-local    # Build for current platform
make build          # Cross-compile for Linux amd64
make install        # Build + install to /usr/local/bin (with version from git)
make lint           # go vet
make test           # go test
make proto          # Regenerate protobuf
make deploy NODE=root@ip  # Deploy minion via SSH
```

## UI Design

- btop-inspired: muted color palette, dense layout, ASCII-only indicators
- Status tags: `[OK]` `[!!]` `[..]` `[--]` `[  ]` instead of Unicode
- Spinners: ora-style line spinner (`-\|/`) for loading states
- ASCII art header on home screen with version + stats
- Breadcrumb navigation in status bar with key hints
- Scrollable viewports for cloud-init view and build detail
- Number keys `[1]-[7]` for quick menu navigation

## Validation

Cloud-init YAML is validated at every entry point:
- CLI add/edit: `ValidateStrict` (rejects invalid YAML, warns on unknown keys)
- TUI edit (after neovim exits): same
- Import/seed: `Validate` (warns, imports anyway)
- Minion builder: `Validate` (rejects, fails build)

## Proxmox Environment Check

Minion commands (`serve`, `connect`) verify: Linux OS, `pveversion` on PATH, `qm` on PATH, `/etc/pve` exists.

## Self-Update

`pvectgen update` checks GitHub releases, downloads the right binary for OS/arch, replaces itself. `pvectgen version` shows update hint when newer version exists.
