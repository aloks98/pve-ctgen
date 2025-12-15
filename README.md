# Proxmox VE Cloud-Init Template Generator (pve-ctgen)

A Go-based Terminal User Interface (TUI) application that automates the creation of Proxmox VE virtual machine templates from official cloud images. It downloads images, verifies checksums, configures VMs using cloud-init, and converts them to reusable templates.

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Supported Operating Systems](#supported-operating-systems)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
  - [OS List Configuration](#os-list-configuration)
  - [Steps Configuration](#steps-configuration)
  - [Cloud-Init Configuration](#cloud-init-configuration)
- [Usage](#usage)
- [How It Works](#how-it-works)
- [Project Structure](#project-structure)
- [Customization](#customization)
- [Troubleshooting](#troubleshooting)

## Features

- **Interactive TUI**: Real-time progress tracking with a hierarchical tree view showing all images and their processing steps
- **Multi-Distribution Support**: Pre-configured for Ubuntu, Debian, Fedora, Rocky Linux, and AlmaLinux
- **Intelligent Checksum Verification**: Supports multiple checksum formats (SHA512, SHA256, SHA1, MD5) and auto-detects algorithm
- **Download Caching**: Skips downloads when local files match remote checksums
- **Live Command Output**: Streams stdout/stderr in real-time during command execution
- **Error Resilience**: Graceful error handling with per-image error logging
- **Template-Based Commands**: Configurable command sequences with variable substitution
- **Cloud-Init Integration**: Full cloud-init support for VM initialization

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         TUI Application                              │
├─────────────────┬───────────────────────────────────────────────────┤
│                 │                                                    │
│   Steps Tree    │    Step View (current step name)                  │
│   (Progress)    │───────────────────────────────────────────────────│
│                 │    Command View (executing command)               │
│   ❔ Image 1    │───────────────────────────────────────────────────│
│   ├─ ✅ Step 1  │                                                    │
│   ├─ ⚙️ Step 2  │    Output View (live command output)              │
│   └─ ❔ Step 3  │                                                    │
│   ❔ Image 2    │                                                    │
│                 │                                                    │
└─────────────────┴───────────────────────────────────────────────────┘
```

### Package Structure

| Package | Description |
|---------|-------------|
| `main.go` | Application entry point, TUI layout setup |
| `pkg/types` | Data structures for Image, Step, and UI components |
| `pkg/ui` | UI components, tree building, and status management |
| `pkg/generator` | Main orchestration logic and workflow execution |
| `pkg/utils` | Utilities for downloads, checksums, and command execution |
| `pkg/style` | Terminal color styling |

## Supported Operating Systems

The tool comes pre-configured with the following cloud images:

| VM ID | Name | Distribution | Version | Cloud-Init Config |
|-------|------|--------------|---------|-------------------|
| 8201 | ubuntu2404 | Ubuntu | 24.04 LTS | ubuntu.yaml |
| 8202 | debian13 | Debian | 13 (Trixie) | debian.yaml |
| 8203 | debian12 | Debian | 12 (Bookworm) | debian.yaml |
| 8204 | alma10 | AlmaLinux | 10 | almalinux.yaml |
| 8205 | alma9 | AlmaLinux | 9 | almalinux.yaml |
| 8206 | fedora42 | Fedora | 42 | fedora.yaml |
| 8207 | rocky10 | Rocky Linux | 10 | rocky.yaml |
| 8208 | rocky9 | Rocky Linux | 9 | rocky.yaml |

## Prerequisites

- **Proxmox VE**: A running Proxmox VE node (tested on PVE 7.x and 8.x)
- **Root Access**: Root/sudo privileges on the Proxmox node
- **Storage**:
  - `/var/lib/vz/template/iso` for downloaded images
  - `/var/lib/vz/snippets` for cloud-init configurations
  - `local-lvm` storage for VM disks
- **Network**: Internet access for downloading cloud images
- **Go 1.24+**: Required only if building from source

## Installation

### Option 1: Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/pve-ctgen.git
cd pve-ctgen

# Build for Linux (cross-compile if on macOS/Windows)
make build

# Copy to Proxmox node
scp -r bin/ root@proxmox-host:/root/pve-ctgen
```

### Option 2: Manual Build

```bash
# Build the binary
GOOS=linux GOARCH=amd64 go build -o bin/generate main.go

# Copy configuration files
cp -r config/ bin/
cp -r cloudinit/ bin/

# Deploy to Proxmox
scp -r bin/ root@proxmox-host:/root/pve-ctgen
```

## Configuration

### OS List Configuration

The `config/os_list.json` file defines which OS images to template:

```json
[
  {
    "id": 8201,
    "name": "ubuntu2404",
    "url": "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
    "checksum_url": "https://cloud-images.ubuntu.com/noble/current/SHA256SUMS",
    "tags": "ubuntu,cloudinit,template",
    "vendor": "ubuntu.yaml"
  }
]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | integer | Yes | Unique Proxmox VM ID for the template |
| `name` | string | Yes | Name for the downloaded image and template |
| `url` | string | Yes | Direct download URL for the qcow2/img cloud image |
| `checksum_url` | string | No | URL to checksum file for verification |
| `tags` | string | Yes | Comma-separated Proxmox tags |
| `vendor` | string | Yes | Cloud-init config filename in `cloudinit/` directory |

### Steps Configuration

The `config/steps.json` file defines the template creation workflow:

```json
[
  {
    "name": "Destroy VM if exists",
    "command": "qm destroy {{.ID}} --purge || true"
  },
  {
    "name": "Resize disk to 32GB",
    "command": "qemu-img resize {{.FilePath}} 32G"
  },
  {
    "name": "Create VM",
    "command": "qm create {{.ID}} --name {{.Name}} --memory 1024 --cores 2 --cpu host --bios ovmf --machine q35 --net0 virtio,bridge=vmbr0"
  },
  {
    "name": "Import disk",
    "command": "qm importdisk {{.ID}} {{.FilePath}} local-lvm"
  },
  {
    "name": "Set disk options",
    "command": "qm set {{.ID}} --virtio0 local-lvm:vm-{{.ID}}-disk-0,discard=on"
  },
  {
    "name": "Set boot options",
    "command": "qm set {{.ID}} --boot c --bootdisk virtio0"
  },
  {
    "name": "Set cloud-init drive",
    "command": "qm set {{.ID}} --scsi1 local-lvm:cloudinit"
  },
  {
    "name": "Set IP configuration",
    "command": "qm set {{.ID}} --ipconfig0 ip=dhcp"
  },
  {
    "name": "Set tags",
    "command": "qm set {{.ID}} --tags {{.Tags}}"
  },
  {
    "name": "Set credentials",
    "command": "qm set {{.ID}} --cipassword yourpassword --ciuser root"
  },
  {
    "name": "Set cloud-init user data",
    "command": "qm set {{.ID}} --cicustom user=local:snippets/{{.Vendor}}"
  },
  {
    "name": "Convert to template",
    "command": "qm template {{.ID}}"
  }
]
```

#### Available Template Variables

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `{{.ID}}` | VM ID from os_list.json | `8201` |
| `{{.Name}}` | Image name from os_list.json | `ubuntu2404` |
| `{{.Tags}}` | Tags from os_list.json | `ubuntu,cloudinit,template` |
| `{{.Vendor}}` | Cloud-init config filename | `ubuntu.yaml` |
| `{{.FilePath}}` | Path to downloaded image | `base.qcow2` |

### Cloud-Init Configuration

Cloud-init YAML files in the `cloudinit/` directory configure the VM on first boot:

```yaml
#cloud-config
hostname: ubuntu-template

# System updates
package_update: true
package_upgrade: true

# Locale and timezone
locale: en_US.UTF-8
timezone: America/New_York

# User configuration
users:
  - name: admin
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - ssh-ed25519 AAAA... your-key-here

# Package installation
packages:
  - qemu-guest-agent
  - curl
  - git
  - vim

# Post-install commands
runcmd:
  - systemctl enable qemu-guest-agent
  - systemctl start qemu-guest-agent
```

#### Distribution-Specific Notes

| Distribution | Package Manager | Sudo Group | Notes |
|--------------|-----------------|------------|-------|
| Ubuntu/Debian | apt | `sudo` | Uses APT repositories |
| Fedora | dnf | `wheel` | Uses DNF repositories |
| Rocky/AlmaLinux | dnf | `wheel` | Enable CRB repo for additional packages |

## Usage

### Running the Generator

```bash
# SSH into Proxmox node
ssh root@proxmox-host

# Navigate to installation directory
cd /root/pve-ctgen

# Run the generator
./generate
```

### TUI Controls

| Key | Action |
|-----|--------|
| `ESC` | Exit application (with confirmation) |
| Arrow keys | Navigate tree view |
| `Enter` | Expand/collapse tree nodes |

### Status Indicators

| Icon | Status |
|------|--------|
| ❔ | Pending |
| ⚙️ | Running |
| ✅ | Success |
| ❌ | Failed |
| ➖ | Skipped |

## How It Works

### Execution Flow

```
1. Load Configuration
   └── Read os_list.json and steps.json

2. Initialize UI
   └── Build tree view with all images and steps

3. Create Directories
   ├── /var/lib/vz/template/iso
   ├── /var/lib/vz/snippets
   └── logs/

4. Process Each Image
   │
   ├── Download Phase
   │   ├── Check for existing local file
   │   ├── Fetch and parse remote checksum
   │   ├── Calculate local file checksum
   │   ├── Compare checksums
   │   └── Download if mismatch or missing
   │
   ├── Prepare Phase
   │   └── Copy downloaded image to base.qcow2
   │
   ├── Execute Steps
   │   ├── Copy cloud-init config to /var/lib/vz/snippets/
   │   ├── For each step in steps.json:
   │   │   ├── Replace template variables
   │   │   ├── Execute command
   │   │   ├── Stream output to UI
   │   │   └── Update status (✅/❌)
   │   └── Skip remaining steps on failure
   │
   └── Cleanup Phase
       └── Remove base.qcow2

5. Display Summary
   └── List any failed images
```

### Checksum Format Support

The tool intelligently parses multiple checksum file formats:

| Format | Example | Used By |
|--------|---------|---------|
| Standard | `abc123... filename.img` | Ubuntu, Debian |
| Fedora | `## filename.img`<br>`SHA256: abc123...` | Fedora |
| Rocky/Alma | `filename.img (SHA256) = abc123...` | Rocky, AlmaLinux |
| Single Value | `abc123...` | Various |

## Project Structure

```
pve-ctgen/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── go.sum                  # Dependency checksums
├── Makefile                # Build automation
├── README.md               # This documentation
│
├── config/
│   ├── os_list.json        # OS image definitions
│   └── steps.json          # Template creation steps
│
├── cloudinit/
│   ├── ubuntu.yaml         # Ubuntu cloud-init config
│   ├── debian.yaml         # Debian cloud-init config
│   ├── fedora.yaml         # Fedora cloud-init config
│   ├── rocky.yaml          # Rocky Linux cloud-init config
│   └── almalinux.yaml      # AlmaLinux cloud-init config
│
├── pkg/
│   ├── types/
│   │   └── types.go        # Data structures
│   ├── ui/
│   │   └── ui.go           # UI components
│   ├── generator/
│   │   └── generator.go    # Main workflow logic
│   ├── utils/
│   │   └── utils.go        # Utility functions
│   └── style/
│       └── style.go        # Terminal styling
│
├── bin/                    # Build output (generated)
└── logs/                   # Error logs (generated at runtime)
```

## Customization

### Adding a New OS

1. **Add image definition** to `config/os_list.json`:
```json
{
  "id": 8210,
  "name": "centos-stream9",
  "url": "https://cloud.centos.org/centos/9-stream/x86_64/images/CentOS-Stream-GenericCloud-9-latest.x86_64.qcow2",
  "checksum_url": "https://cloud.centos.org/centos/9-stream/x86_64/images/CentOS-Stream-GenericCloud-9-latest.x86_64.qcow2.SHA256SUM",
  "tags": "centos,cloudinit,template",
  "vendor": "centos.yaml"
}
```

2. **Create cloud-init config** in `cloudinit/centos.yaml`:
```yaml
#cloud-config
hostname: centos-template
package_update: true
packages:
  - qemu-guest-agent
runcmd:
  - systemctl enable --now qemu-guest-agent
```

### Modifying VM Hardware

Edit `config/steps.json` to change VM specifications:

```json
{
  "name": "Create VM",
  "command": "qm create {{.ID}} --name {{.Name}} --memory 2048 --cores 4 --cpu host --bios ovmf --machine q35 --net0 virtio,bridge=vmbr0"
}
```

### Changing Storage

Modify the storage target in `steps.json`:

```json
{
  "name": "Import disk",
  "command": "qm importdisk {{.ID}} {{.FilePath}} zfs-pool"
}
```

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Permission denied | Not running as root | Run with `sudo` or as root user |
| Storage not found | Wrong storage name in steps.json | Verify storage exists with `pvesm status` |
| Download fails | Network/firewall issues | Check connectivity to image URLs |
| Checksum mismatch | Corrupted download or outdated URL | Delete local file and retry |
| VM ID conflict | ID already in use | Change ID in os_list.json or delete existing VM |

### Error Logs

Detailed error logs are written to the `logs/` directory:
```bash
# View error log for a specific image
cat logs/ubuntu2404.error.log
```

### Debug Mode

To see raw command output, check the live output panel in the TUI or review error logs.

### Verifying Templates

After generation, verify templates in Proxmox:
```bash
# List all templates
qm list | grep template

# Check template configuration
qm config 8201
```

## Warning

**Data Loss Risk**: This application will **permanently destroy any existing Proxmox VMs** that share an ID with entries in `config/os_list.json`. This is intentional to ensure clean template creation. Always verify VM IDs before running.

## License

MIT License - See LICENSE file for details.
