# Upgrading existing installations

## Manager and minion transport

Upgrade both ends together. New connections require TLS; there is no plaintext fallback. The minion provisions a certificate/key next to its configuration on first startup. Enrollment tokens contain the SHA-256 certificate fingerprint as well as the API key. Treat tokens as credentials.

After upgrading the minion, run `pvectgen minion connect` on that node. Re-enroll its manager entry with the new token (remove the old entry, then `manager node add --token ...`). Existing build records remain in the manager's data directory. CA-managed certificates can instead use absolute `tls_cert`/`tls_key` paths and normal system CA validation when no fingerprint is configured. Certificate rotation requires re-enrollment for pinned clients.

API keys, TLS keys, enrollment configuration and node credentials are written with mode 0600. Auto-generated certificates expire after two years. Provision a replacement identity and re-enroll before expiration.

## Node storage and profiles

See [the complete minion example](../configs/minion.example.yaml). Profiles select image storage, snippet storage/path, bridge, disk size and disk format. Select one through `default_profile`, a template's `profile` field, or `manager build run --profile NAME`. Launch accepts `--profile NAME` too.

Run:

```sh
pvectgen manager doctor --node NODE
pvectgen manager doctor --node NODE --profile PROFILE --json
```

The snippet storage must advertise `snippets` content, and `snippets_path` must equal that storage's directory plus `/snippets`. Enable the content type through Proxmox storage settings while preserving the storage's existing content types, or select a dedicated snippet storage. Preflight reports mismatches; it never edits Proxmox storage configuration. The host used during testing had a `local` storage that did not advertise snippets, so tests used a separate storage.

Builds now validate image format and use Proxmox's reported imported volume instead of assuming `vm-ID-disk-1`. Existing custom build steps remain stored. To adopt the new default steps, review/backup `steps.yaml`, then run `pvectgen manager steps reset`. Shell placeholders are quoted as literal values. Legacy default destruction is superseded by the guarded overwrite operation.

Rebuilding an existing template requires `manager build run --overwrite`. Builds never replace ordinary VMs. Launch replacement requires `vm launch --overwrite` and never replaces templates, containers or remote-node guests. A build/launch failure can leave a partial new VM for inspection; it does not automatically delete it. Resolve or remove that partial VM explicitly before retrying.

## Guest defaults

Bundled defaults use locked passwords and allow root public-key SSH login. Titan Air and Fatman keys are managed in `config/guest-ssh-keys.pub`; run `make guest-keys` after editing it to update both users in all cloud-init and Flatcar files. The generated YAML remains standalone and can be used without the manager.

Node profile `ssh_authorized_keys` are merged and deduplicated with bundled keys by default (`ssh_keys_mode: merge`). Set `ssh_keys_mode: replace` to deliberately replace or revoke them; an empty replacement list removes the managed users' key lists. Cloud-init profiles apply to `admin` and to `root` only when `disable_root: false`; Ignition profiles apply to `core` and an explicitly configured `root` user. Custom configurations that disable root remain disabled. Optional `timezone` and `apt_proxy` settings also live in the profile; use the proxy only for appropriate Debian/Ubuntu profiles.

Docker is now opt-in: set `install_docker: true` on a cloud-init node profile. The bundled installer adds the Docker repository, installs Engine/Compose, enables the service, and adds admin to the Docker group. Base guests retain Fish, shell aliases, and the custom MOTD without installing Docker. Custom cloud-init files must supply `/usr/local/sbin/pvectgen-install-docker` in `write_files` to opt in. Flatcar retains containerd and rejects this option.

Full first-boot upgrades are disabled in bundled configs. A cloud-init profile can set `package_upgrade: true` (or explicitly false for a custom config); omission preserves the source file. `package_cleanup: true` opts into weekly package autoremove, and `journal_retention_days: 7` opts into weekly journal rotation/vacuum. Zero keeps the distro's journal policy. These independent options generate a systemd timer, so minimal images do not need cron. The `/usr/local/sbin/pvectgen-maintenance` and `pvectgen-maintenance.{service,timer}` paths are reserved for this policy. Flatcar rejects these cloud-init-specific options.

Existing imported guest files are preserved by ordinary `manager import`. Review and update them separately, or back up customizations before `manager import --force`, which replaces bundled entries. Existing deployed guests are not modified by editing the defaults. Static networking for Ignition must be supplied in the Butane configuration; launch rejects unsupported static overrides rather than silently ignoring them.

Fedora defaults use [Docker's DNF5 repository command](https://docs.docker.com/engine/install/fedora/) and the [English locale package](https://packages.fedoraproject.org/pkgs/glibc/glibc-langpack-en/). Flatcar uses the official, version-pinned [Proxmox image](https://www.flatcar.org/docs/latest/deploy/virt-options/proxmoxve/) and its SHA-512 digest. Image versions are otherwise unchanged in this phase.

## Jobs and command behavior

Jobs continue on the minion after a client disconnects. Reconnect with `manager build watch BUILD_ID`; request cancellation with `manager build cancel BUILD_ID`, then use watch to confirm the final status. A minion restart marks unfinished jobs failed; it never automatically repeats their destructive operations. Journals live in `jobs_dir` (default `/var/lib/pvectgen/jobs`) and have a 64 MiB per-job limit. Retention/garbage collection is not automatic.

The manager stores a replay cursor and persists logs and terminal status for both CLI and TUI builds. A disconnected stream is not reported as a successful build. Failed builds, launches, health checks and doctor checks return a nonzero exit status. Fleet `--dry-run` now connects to referenced nodes and validates resources without creating VMs; resource conflicts are reported as failures.

`manager import` and `manager steps reset` use defaults embedded in the binary and work from any directory. Explicit import paths still work. `default_node` applies when `--node` is omitted. `version` is offline. `update [vMAJOR.MINOR.PATCH]` verifies the release checksum and atomically replaces the executable; restart a running minion service afterward. The update must have write access to the executable directory.
