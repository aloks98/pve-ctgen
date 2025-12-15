# CLAUDE.md - Project Context for AI Assistants

This file provides context for Claude and other AI assistants when working on the pve-ctgen codebase.

## Project Overview

**pve-ctgen** (Proxmox VE Cloud-Init Template Generator) is a Go-based TUI application that automates creating Proxmox VE virtual machine templates from cloud images. It downloads images, verifies checksums, configures VMs with cloud-init, and converts them to templates.

## Tech Stack

- **Language**: Go 1.24+
- **TUI Framework**: `github.com/rivo/tview` (built on `tcell`)
- **Terminal Colors**: `github.com/fatih/color`
- **Target Platform**: Linux (Proxmox VE nodes)
- **Build**: Cross-compiled from any platform via Makefile

## Project Structure

```
pve-ctgen/
├── main.go                 # Entry point - TUI layout and app initialization
├── pkg/
│   ├── types/types.go      # Data structures (Image, Step, UIStep, UIImage)
│   ├── ui/ui.go            # UI components and tree management
│   ├── generator/generator.go  # Main orchestration and workflow
│   ├── utils/utils.go      # Downloads, checksums, command execution
│   └── style/style.go      # Terminal color helpers
├── config/
│   ├── os_list.json        # OS image definitions (id, name, url, checksum_url, tags, vendor)
│   └── steps.json          # Template creation commands (name, command)
└── cloudinit/
    ├── ubuntu.yaml         # Ubuntu cloud-init config
    ├── debian.yaml         # Debian cloud-init config
    ├── fedora.yaml         # Fedora cloud-init config
    ├── rocky.yaml          # Rocky Linux cloud-init config
    └── almalinux.yaml      # AlmaLinux cloud-init config
```

## Key Data Structures

```go
// pkg/types/types.go

type Image struct {
    ID          int    `json:"id"`           // Proxmox VM ID
    Name        string `json:"name"`         // Image/template name
    URL         string `json:"url"`          // Download URL
    ChecksumURL string `json:"checksum_url"` // Optional checksum file URL
    Tags        string `json:"tags"`         // Comma-separated Proxmox tags
    Vendor      string `json:"vendor"`       // Cloud-init config filename
}

type Step struct {
    Name    string `json:"name"`    // Display name
    Command string `json:"command"` // Shell command with {{.Var}} placeholders
}
```

## Core Workflow

1. **Load configs** from `config/os_list.json` and `config/steps.json`
2. **Build UI tree** with images as parent nodes, steps as children
3. **For each image**:
   - Download & verify checksum (supports SHA512, SHA256, SHA1, MD5)
   - Copy image to `base.qcow2`
   - Copy cloud-init config to `/var/lib/vz/snippets/`
   - Execute each step command with template variable substitution
   - Clean up `base.qcow2`
4. **Display summary** of failed/successful images

## Template Variables

Commands in `steps.json` support these placeholders:

| Variable | Source | Example |
|----------|--------|---------|
| `{{.ID}}` | Image.ID | `8201` |
| `{{.Name}}` | Image.Name | `ubuntu2404` |
| `{{.Tags}}` | Image.Tags | `ubuntu,cloudinit` |
| `{{.Vendor}}` | Image.Vendor | `ubuntu.yaml` |
| `{{.FilePath}}` | Hardcoded | `base.qcow2` |

## Checksum Parsing

The `GetExpectedChecksum()` function in `pkg/utils/utils.go` handles multiple formats:

1. **Standard**: `checksum  filename`
2. **Fedora**: `## filename` followed by `SHA256: checksum`
3. **Rocky/Alma**: `filename (ALGORITHM) = checksum`
4. **Single value**: Just the checksum string

Algorithm detection is based on checksum length (128=SHA512, 64=SHA256, 40=SHA1, 32=MD5).

## UI Components

```go
// pkg/ui/ui.go

type UI struct {
    App         *tview.Application
    StepsTree   *tview.TreeView   // Left panel - progress tree
    StepView    *tview.TextView   // Top-right - current step name
    CommandView *tview.TextView   // Middle-right - executing command
    OutputView  *tview.TextView   // Bottom-right - live output
}
```

### Status Icons

- `❔` Pending (yellow)
- `⚙️` Running (yellow)
- `✅` Success (green)
- `❌` Failed (red)
- `➖` Skipped (gray)

## Important Functions

### pkg/generator/generator.go
- `Run(ui *ui.UI)` - Main entry point, orchestrates the entire workflow

### pkg/utils/utils.go
- `LoadImages(path)` - Parse os_list.json
- `LoadSteps(path)` - Parse steps.json
- `HandleDownloadAndChecksum()` - Download logic with checksum verification
- `GetExpectedChecksum()` - Parse various checksum file formats
- `CalculateFileChecksum()` - Compute file checksum
- `ExecuteCommands()` - Run templated commands for an image
- `RunCommandWithStreaming()` - Execute shell command with live output

### pkg/ui/ui.go
- `NewUI()` - Initialize all UI components
- `BuildUITree()` - Construct tree from images/steps
- `UpdateNodeStatus()` - Change node icon and color

## Common Tasks

### Adding a new OS
1. Add entry to `config/os_list.json` with unique ID
2. Create cloud-init YAML in `cloudinit/` if needed
3. Set `vendor` field to the YAML filename

### Modifying VM hardware
Edit the "Create VM" step in `config/steps.json`:
```json
{
  "name": "Create VM",
  "command": "qm create {{.ID}} --name {{.Name}} --memory 2048 --cores 4 ..."
}
```

### Adding a new step
Append to `config/steps.json`:
```json
{
  "name": "Step Name",
  "command": "command --with {{.ID}} placeholders"
}
```

### Changing default storage
Modify `local-lvm` references in `config/steps.json` to your storage name.

## Build Commands

```bash
make build   # Cross-compile for Linux, package to bin/
make lint    # Run golint
```

## File Paths Used at Runtime

| Path | Purpose |
|------|---------|
| `/var/lib/vz/template/iso/` | Downloaded cloud images |
| `/var/lib/vz/snippets/` | Cloud-init configs |
| `./logs/` | Per-image error logs |
| `./base.qcow2` | Temporary working copy of image |

## Error Handling Pattern

- Errors during image processing mark the image as failed
- Remaining steps for that image are marked as "skipped"
- Detailed errors logged to `logs/{imagename}.error.log`
- Processing continues to next image

## Code Style Notes

- Uses goroutines for non-blocking UI updates
- Command output streaming uses separate goroutines for stdout/stderr
- Progress updates rate-limited to 100ms for UI responsiveness
- All Proxmox commands use `qm` CLI tool

## Testing Locally

The binary must run on a Proxmox node with:
- Root privileges
- `qm` command available
- Network access to image URLs
- `local-lvm` storage (or modified config)

## Dependencies

Direct:
- `github.com/rivo/tview` - TUI framework
- `github.com/gdamore/tcell/v2` - Terminal handling
- `github.com/fatih/color` - Colored output
