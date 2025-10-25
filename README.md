# Proxmox VE Cloud-Init Template Generator

This project automates the creation of Proxmox VE virtual machine templates from official cloud images. It downloads specified images, creates a new VM, configures it using cloud-init, and then converts the VM into a reusable template.

## How it Works

The application is a single Go binary that performs the following steps when executed on a Proxmox node:

1.  **Reads Configuration**: It parses the `os_list.json` file to get a list of OS images to process. Each entry in this JSON file defines the VM ID, name, download URL, Proxmox tags, and the corresponding cloud-init configuration file.

2.  **Ensures Paths**: It creates the necessary directories (`/var/lib/vz/template/iso` for images and `/var/lib/vz/snippets` for cloud-init files) on the Proxmox node if they don't already exist.

3.  **Downloads Image**: For each OS in the list, it checks if the cloud image already exists. If not, it downloads the image from the specified URL and displays a progress bar.

4.  **VM Creation and Configuration**: It then automates the VM creation process by executing a series of shell commands:
    *   It destroys any existing VM with the same ID to ensure a clean slate.
    *   Copies the appropriate cloud-init configuration file (e.g., `ubuntu.yaml`) to the Proxmox snippets directory.
    *   Resizes the downloaded disk image to a default of 32GB using `qemu-img`.
    *   Creates a new VM using `qm create` with a predefined set of hardware configurations (CPU, memory, network, etc.).
    *   Imports the resized disk to the VM's storage (`local-lvm`).
    *   Sets various VM options using `qm set`, including boot order, cloud-init drive, networking (DHCP), user credentials, and tags.

5.  **Template Conversion**: Once the VM is fully configured, it converts the VM into a template using `qm template`.

6.  **Cleanup**: It removes the temporary disk image file (`base.qcow2`) used during the creation process.

## Prerequisites

*   A running Proxmox VE node.
*   Go (if you wish to build the binary yourself).
*   Sudo/root privileges on the Proxmox node to run the binary, as it needs to execute `qm` commands and write to system directories.

## Usage

### 1. Configuration

Modify the `os_list.json` file to define the OS images you want to create templates for.

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
*   `tags`: Comma-separated tags to apply to the Proxmox template.
*   `vendor`: The name of the cloud-init configuration file located in the `cloudinit/` directory.

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
├── os_list.json
└── cloudinit/
    ├── almalinux.yaml
    ├── debian.yaml
    ├── fedora.yaml
    ├── motd.sh
    ├── rocky.yaml
    └── ubuntu.yaml
```

### 3. Deployment and Execution

1.  Copy the entire `bin` directory to your Proxmox VE node.
    ```sh
    scp -r bin/ user@proxmox-host:/root/
    ```

2.  SSH into your Proxmox node, navigate to the directory, and run the executable. It is recommended to run as root or with `sudo`.
    ```sh
    ssh user@proxmox-host
    cd /root/bin
    sudo ./generate
    ```

The application will then start the process of downloading images and creating the templates.

## Project Structure

```
.
├── generate_templates.go  # Main application source code.
├── os_list.json           # JSON file defining the OS images to be templated.
├── cloudinit/             # Directory containing cloud-init user-data files.
├── Makefile               # Makefile for building the application.
└── README.md              # This file.
```
