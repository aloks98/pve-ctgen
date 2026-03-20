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
| Manager storage | SQLite via `modernc.org/sqlite` (pure Go) |
| Config format | YAML via `gopkg.in/yaml.v3` |
| Auth | API key in gRPC metadata |
| Proxmox interaction | `qm`, `pvesh` shell commands |
| CI/CD | GitHub Actions + GoReleaser |

## Project Structure

```
pve-ctgen/
├── cmd/pvectgen/main.go              # Single binary entry point (Cobra root)
├── internal/
│   ├── shared/                       # Code shared by manager and minion
│   │   ├── models/models.go          # All data models
│   │   ├── checksum/checksum.go      # Checksum parsing and verification
│   │   ├── cloudinit/validate.go     # YAML validation for cloud-init configs
│   │   ├── download/download.go      # HTTP download with progress callback
│   │   ├── token/token.go            # Connection token encode/decode
│   │   └── fileutil/fileutil.go      # File utilities
│   ├── manager/
│   │   ├── cli/                      # All Cobra commands
│   │   ├── tui/
│   │   │   ├── app.go               # BubbleTea root model + build orchestration
│   │   │   ├── views/               # home, nodes, cloudinit, templates, steps, builds, buildselect, buildprogress, vmlaunch
│   │   │   ├── components/           # table, form, logviewer, statusbar
│   │   │   └── styles/styles.go     # Lipgloss theme
│   │   ├── store/                    # SQLite CRUD for all entities
│   │   ├── grpc/client.go           # gRPC client to Minion
│   │   └── config/config.go         # Manager YAML config
│   └── minion/
│       ├── cli/                      # serve, connect + Proxmox environment check
│       ├── server/server.go         # gRPC server + API key interceptor
│       ├── executor/executor.go     # Channel-based shell command execution
│       ├── builder/builder.go       # Build orchestration
│       └── config/config.go         # Minion YAML config + auto node name + API key gen
├── proto/pvectgen/v1/               # Protobuf service definition + generated code
├── configs/                         # Example configs + systemd unit
├── scripts/install-minion.sh        # One-line minion installer
├── config/                          # Seed data (os_list.json, steps.json)
├── cloudinit/                       # Seed cloud-init configs
├── .github/workflows/               # CI + release pipelines
├── .goreleaser.yml                  # Multi-platform release config
└── Makefile
```

## CLI Command Tree

```
pvectgen
├── manager
│   ├── tui                          # Interactive TUI
│   ├── cloudinit list|add|show|edit|remove
│   ├── template list|add|show|edit|remove
│   ├── steps list|add|edit|remove|reset
│   ├── node list|add|remove|health
│   ├── build run|cancel
│   ├── builds list|show|logs
│   ├── vm launch|list
│   └── import
├── minion
│   ├── serve [--config path]
│   └── connect
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

Auth: API key in `x-api-key` gRPC metadata, validated by unary+stream interceptor.

## Key Workflows

### Minion Onboarding
1. Install on Proxmox: `curl -fsSL .../install-minion.sh | bash`
2. Auto-detects hostname, generates API key, outputs connection token
3. On workstation: `pvectgen manager node add --token <token> --display-name "prod"`

### Build Flow (TUI)
1. Home → New Build → select templates (space/a) → enter → select node → enter
2. App connects to minion via gRPC, streams BuildEvents back
3. Live split-pane progress: step tree (left) + logs (right)
4. Results persisted to SQLite build history

### VM Launch (TUI)
1. Select node → fetches templates from minion via `pvesh` API
2. Select template → configure VM (text inputs + select toggles)
3. Static IP shows extra fields (address + gateway)
4. Launches via gRPC LaunchVM

## Build Commands

```bash
make build-local    # Build for current platform
make build          # Cross-compile for Linux amd64
make lint           # go vet
make test           # go test
make proto          # Regenerate protobuf
make deploy NODE=root@ip  # Deploy minion via SSH
make snapshot       # GoReleaser local build (all platforms)
make release VERSION=v1.0.0  # Tag + push (triggers GitHub Actions)
```

## Configuration

- **Manager**: `~/.config/pvectgen/config.yaml` (db_path, default_node)
- **Minion**: `/etc/pvectgen/minion.yaml` (listen_address, node_name, api_key, paths)

## Validation

Cloud-init YAML is validated at every entry point:
- CLI add/edit: `ValidateStrict` (rejects invalid YAML, warns on unknown keys)
- TUI add/edit: same, shown in status bar
- Import/seed: `Validate` (warns, imports anyway)
- Minion builder: `Validate` (rejects, fails build)

## Proxmox Environment Check

Minion commands (`serve`, `connect`) verify: Linux OS, `pveversion` on PATH, `qm` on PATH, `/etc/pve` exists.
