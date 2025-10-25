# Proxmox VE Cloud-Init Template Generator

This project automates the creation of Proxmox VE virtual machine templates from official cloud images. It downloads specified images, creates a new VM, configures it using cloud-init, and then converts the VM into a reusable template.

## How it Works

The application is a Go-based Terminal User Interface (TUI) program that automates the creation of Proxmox VE virtual machine templates. It runs on a Proxmox node and performs the following steps for each image defined in its configuration:

1.  **Interactive TUI**: Presents an interactive terminal interface using `tview`, displaying a tree view of the overall progress, the currently executing command, and live streaming output from each step.

2.  **Reads Configuration**: It loads image definitions from `config/os_list.json` and a sequence of shell commands (steps) from `config/steps.json`. These JSON files allow for flexible and dynamic template generation.

3.  **Ensures Paths**: Verifies and creates necessary directories on the Proxmox node: `/var/lib/vz/template/iso` (for downloaded images), `/var/lib/vz/snippets` (for cloud-init configuration files), and a local `logs/` directory for error logging.

4.  **Download and Robust Checksum Verification**: For each OS image:
    *   It first checks if the image already exists locally.
    *   If a `checksum_url` is provided in `config/os_list.json`, it downloads the checksum file.
    *   The application intelligently parses various checksum formats (e.g., standard, Fedora, Rocky Linux, or single-value files) to extract the expected checksum and algorithm (SHA512, SHA256, SHA1, MD5).
    *   It calculates the checksum of the local image file.
    *   If the local checksum matches the expected one, the download is skipped. Otherwise, or if checksum verification fails, the image is downloaded from the specified URL.
    *   Download progress is displayed live in the TUI, with updates rate-limited to maintain UI responsiveness.

5.  **Dynamic Command Execution**: The application proceeds to execute a series of shell commands defined in `config/steps.json`. These commands are dynamically templated using placeholders like `{{.ID}}`, `{{.Name}}`, `{{.Tags}}`, `{{.Vendor}}`, and `{{.FilePath}}` (referring to the downloaded image).
    *   **Pre-existing VM Handling**: The first step typically includes a command to destroy any existing VM with the same ID, ensuring a clean state for template creation.
    *   **Cloud-Init Setup**: It copies the appropriate cloud-init configuration file (e.g., `ubuntu.yaml` from the `cloudinit/` directory) to the Proxmox snippets directory (`/var/lib/vz/snippets/`).
    *   **VM Creation and Configuration**: Commands are executed to resize the disk, create a new VM, import the disk, set various VM options (e.g., boot order, cloud-init drive, network, user credentials, tags), and finally convert the VM into a template.
    *   **Live Output Streaming**: `stdout` and `stderr` from each executed command are streamed live to the TUI's output panel. This streaming is handled concurrently in separate goroutines to prevent deadlocks and maintain UI responsiveness.
    *   **Step Status**: The status of each step (running, success, failed, skipped) is visually updated in the progress tree.
    *   **Error Handling**: If any command step fails, subsequent steps for that image are automatically skipped.

6.  **Error Logging**: Detailed error messages for any failures during image processing or command execution are logged to individual files in the `logs/` directory (e.g., `logs/ubuntu-22.04.error.log`).

7.  **Cleanup**: After all steps for an image are completed or skipped, the temporary base disk image file (`base.qcow2`) is removed.

8.  **Final Status**: The overall status for each image (success or failure) is indicated in the main progress tree.
## Prerequisites

*   A running Proxmox VE node.
*   Go (if you wish to build the binary yourself).
*   Sudo/root privileges on the Proxmox node to run the binary, as it needs to execute `qm` commands and write to system directories.

## Usage

### 1. Configuration

Modify the `config/os_list.json` file to define the OS images you want to create templates for.

```json
[
  {
    "id": 9000,
    "name": "ubuntu-22.04",
    "url": "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
    "tags": "ubuntu,cloudinit",
    "vendor": "ubuntu.yaml"
  }
]
```

*   `id`: The unique VM ID for the template.
*   `name`: The name for the downloaded image file.
*   `url`: The direct download URL for the qcow2 cloud image.
*   `checksum_url`: (Optional) The URL to a file containing the checksum for the image. Supports various formats (e.g., standard, Fedora, Rocky Linux, or single-value files).
*   `tags`: Comma-separated tags to apply to the Proxmox template.
*   `vendor`: The name of the cloud-init configuration file located in the `cloudinit/` directory.

Modify the `config/steps.json` file to define the sequence of shell commands for creating Proxmox templates.


You can also customize the cloud-init behavior by editing the corresponding `.yaml` files in the `cloudinit/` directory.

### 2. Building the Application

The included `Makefile` simplifies the build process. It cross-compiles the Go program for Linux and packages it with the necessary configuration files into a `bin` directory.

```sh
make build
```

This will create a `bin` directory with the following structure:

```
bin/
├── generate
└── config/
    ├── os_list.json
    └── steps.json
└── cloudinit/
    ├── almalinux.yaml
    ├── debian.yaml
    ├── fedora.yaml
    ├── motd.sh
    ├── rocky.yaml
    └── ubuntu.yaml
```

### 3. Execution

**⚠️ WARNING: Data Loss Imminent!**

This application will **permanently destroy any existing Proxmox VMs** that share an ID with an entry in your `config/os_list.json` file. This is an intentional feature to ensure a clean slate for template creation. **Ensure you have backups or are certain you want to delete VMs with conflicting IDs before running this tool.**

1.  Copy the entire `bin` directory to your Proxmox VE node.
    ```sh
    scp -r bin/ root@proxmox-host:/root/
    ```

2.  SSH into your Proxmox node, navigate to the directory, and run the executable. **This application must be run as the `root` user.**
    ```sh
    ssh root@proxmox-host
    cd /root/bin
    ./generate
    ```

The application will then start the process of downloading images and creating the templates.

## Project Structure

```
.
├── generate_templates.go  # Main application source code.
├── config/                # Directory containing configuration files.
│   ├── os_list.json       # JSON file defining the OS images to be templated.
│   └── steps.json         # JSON file defining the steps for template generation.
├── cloudinit/             # Directory containing cloud-init user-data files.
├── Makefile               # Makefile for building the application.
└── README.md              # This file.
```
