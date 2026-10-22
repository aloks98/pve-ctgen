# Verification

## Local checks

```sh
go test -race ./...
go vet ./...
make vuln
bash scripts/test-install.sh
for script in scripts/*.sh; do bash -n "$script"; done
```

The HTTP download and concurrent-builder tests use loopback listeners. Run them in an environment that permits local sockets. Build tests replace Proxmox commands with fixtures; ordinary tests never create VMs.

Regression coverage includes rejected destructive requests, checksum formats and conflicting digests, cache corruption/cancellation, concurrent workspaces and store updates, malformed config, literal shell substitution, subprocess cancellation, persistent job replay, truncated-journal recovery, TLS identity/pinning, embedded seed resolution and atomic executable replacement.

## Live Proxmox verification, 2026-10-22/23

Test host: `tokyo` (`192.168.100.1`), Proxmox VE 9.2.3. The existing production minion was kept running. A separate minion listened only on `127.0.0.1:50061`, with its own configuration, certificates, cache, workspace, manager store and journals under `/var/lib/pvectgen-review-test`. A fixture HTTP server listened on loopback port 50062. Dedicated snippet storage was named `pvectgen-review-test`; test disks used `local-lvm`.

Reserved IDs were checked free before testing:

- `998100`: dedicated Debian template built through the streaming API.
- `998101`: full clone, booted with DHCP, one core, 1 GiB RAM and an 8 GiB disk.
- `998102`: absent VM ID used for missing-source and cancellable job tests.

Observed results:

- Pinned TLS enrollment and authenticated health calls succeeded; an invalid API key was rejected.
- Doctor accepted the test profile and rejected a missing storage.
- Verified download, resize, EFI disk creation, import, snippet attachment and template conversion succeeded.
- The clone's guest agent reported `pvectgen-test-guest`; `cloud-init status --long` reported `done`, with no errors or recoverable errors. A guest marker confirmed the hostname in `runcmd`.
- Same-source/destination, missing source with overwrite, existing destination without overwrite, and rebuild without overwrite all failed. The existing guest configuration was unchanged by those refusals.
- Explicit template rebuild and explicit VM replacement succeeded.
- Disconnect left a running job alive. Watch resumed after an event cursor. Cancellation ended the job and its child process. Completed journals replayed after restarting the test minion.

Fleet dry-run also passed against the isolated minion. After testing, both VMs and all associated disks, dedicated snippet storage, test services and test data were removed. The production minion remained active; original running IDs were still `400, 401, 402, 403, 405, 406`.

Final checks passed for Linux/macOS amd64/arm64 compilation, `go vet`, race tests, installer smoke tests and the vulnerability scan (no vulnerabilities found).

The live tests are opt-in:

```sh
go test -c -o /tmp/pvectgen-live.test ./internal/minion/server
# On an isolated fixture node configured as above:
PVECTGEN_LIVE_CONFIG=/var/lib/pvectgen-review-test/minion.yaml \
  /tmp/pvectgen-live.test -test.run TestLiveJobs -test.v
```

`TestLiveJobs` requires a `broken` profile referencing nonexistent storage and fixture URLs `/disk.qcow2` and `/SHA256SUMS` on loopback port 50062. It executes a cancellable shell job against an unused ID and checks the child process. `TestLiveGuards` is a separate opt-in test using `PVECTGEN_LIVE_TEST_NODE`; it requires all three reserved IDs free, creates named diskless fixtures, and deletes only those fixtures.

That initial review exercised only Debian guest boot end to end. The later guest-config verification below covers additional distributions and Flatcar. Alternate storage backends and multi-node fleet creation remain untested.

## TUI overhaul verification, 2026-10-23

The full race suite and `go vet ./...` passed. Added tests exercise workspace sizing
and navigation, focus retention, Unicode tables and paging, form scrolling,
filtered template actions and delete confirmation, build-review validation,
failed/stale preflight results, reviewed-request immutability, duplicate submission,
event replay, log retention/follow, reconnect cursors and cancellation of queued
work. A real local PTY was also used to inspect the rendered workspace.

On `tokyo` (`192.168.100.1`), a temporary binary and scratch manager store ran in a
PTY against an isolated minion on `127.0.0.1:50071`. The smoke test verified:

- Pinned-TLS enrollment and live node health from the Nodes screen.
- Template filtering, selected-template build flow and VM-ID review.
- An intentionally missing profile produced a failed preflight check; Enter did
  not submit a build. No job journals or build-history records were created.
- Resizing between 120 × 36, 80 × 24 and 40 × 12; keyboard navigation to the last
  template-edit field; clean Ctrl+C exit.

The test minion and scratch data were removed. No VMs or storage configuration
were changed. Reconnect and cancellation in this UI phase were tested through
the session coordinator with controlled streams; actual minion cancellation and
replay were covered by the earlier live tests above. A full guest build/launch was
not repeated for the UI overhaul.

The visual fidelity pass was also tested on 2026-10-23. TUI/CLI race tests and
vet passed, including regressions for 200-column table spacing, aligned headers,
and keeping selected template details/actions visible with a long list. Actual
Go-rendered screens were inspected at 120 × 36 and 200 × 58, plus compact sizes.
The isolated remote PTY smoke test above was repeated with `--mac-keys`, the
Ctrl+G byte sent by the Alacritty Command+G mapping, and 200 × 58 resizing.
All checks passed. The physical Mac keyboard/Alacritty configuration could not be
exercised from the VM; that binding must be added on the Mac.

A subsequent screenshot exposed nested ANSI resets returning parts of the canvas
to the terminal's default background. The canvas now restores its default colors
while preserving explicit selection and log colors. Regression tests interpret
the ANSI output and check every cell, including padding, in both true-color
themes and 256-color mode. TUI race tests and vet passed. An isolated local tmux
session verified explicit backgrounds on all 11,600 cells of a 200 × 58 screen
and Ctrl+G navigation. The earlier HTML preview used the canvas color as its own
background, which masked this bug; cell-level terminal checks are necessary.
The isolated remote PTY smoke test on `192.168.100.1` also passed again with this
fix; its temporary binary, minion and scratch store were removed afterward.


## Guest access and provisioning review, 2026-10-23

Local checks cover central-key drift, all bundled cloud-init schemas, shell/Fish syntax, embedded import/Butane resolution, profile key merge/replacement, Docker opt-in, independent upgrade/cleanup/journal settings, unsupported Ignition policies, and repeated profile application. Package-bootstrap command stubs verified that CRB/EPEL setup precedes dependent packages and failures stop the script. The full Go suite, profile/asset race tests, and `go vet ./...` passed.

Live tests used isolated linked clones on Tokyo, IDs `998200`–`998207`, with private snippet storage `pvectgen-guest-review`. The tested cached images were Debian 13, Ubuntu 24.04, AlmaLinux 10, Rocky 9, Fedora 44, and Flatcar 4593.2.1. Fedora 44 exercises the shared Fedora config; the repository's Fedora 43 image was not downloaded or booted during this pass. Other major-version variants sharing the same config were also not separately booted.

- Root and normal-user key login were exercised using Fatman's key; both Titan Air and Fatman fingerprints were checked in guest authorized keys. Titan Air's private key was not available, so its authentication was verified by installed public key only.
- Effective sshd policy allowed root public keys and disabled password/keyboard-interactive authentication.
- Cloud-init reported completion with no errors or recoverable errors on the final tested configurations. Fish and MOTD output were checked. Ubuntu tests exposed an existing `admin` group and an incorrect `sshd.service` name; fresh clones verified both fixes.
- AlmaLinux/Rocky installed their EPEL-dependent packages after repository setup. Cloud-init base guests had no Docker or custom maintenance schedule.
- Debian and Fedora completed Docker-enabled profiles with Engine/Compose running and a journal-only maintenance timer. Rocky's optional installer also completed successfully. Debian's initial Docker CDN download was slow but completed without intervention.
- Flatcar downloaded the Kubernetes v1.35.5 extension; its on-disk SHA-256 matched the pinned release checksum. Containerd's CRI plugins reported `ok`, forwarding sysctls were set, and the packaged kubelet/kubeadm units were present. Root access and the extension persisted after reboot. No Kubernetes cluster was initialized or joined.

Tokyo's ten existing saved guest snippets received a separate SSH-only update: the retired `aloksahoo@mac` key was removed, Titan Air/Fatman were added, and root key login was enabled. Existing `aloks98` users, node keys (`root@tokyo`, `root@kyoto`, `root@osaka`), MOTDs, packages, commands, proxy settings, and per-VM hostnames were preserved. Cloud-init/Ignition schemas were checked and uploaded hashes verified. Original snippets are backed up under `/root/pvectgen-snippet-backup-20261023-021954` on Tokyo. This updates provisioning files, not authorized keys inside existing running VMs. Broader provisioning changes live in the project defaults and require a new build/import to replace legacy snippets.

All eight test clones and their disks, temporary snippet storage, and remote staging files were removed. Production guest IDs `400, 401, 402, 403, 406` remained running and the production minion remained active. The snippet backup is intentionally retained.

## Existing guest SSH access rollout, 2026-10-23

The live guest accounts were subsequently updated through Proxmox guest-agent
execution and `pct exec`. The final inventory contains 17 running VMs and two
running containers across four independent hosts:

| Host | Address | VMs updated | Containers updated |
| --- | --- | --- | --- |
| Tokyo | `192.168.100.1` | 400, 401, 402, 403 | 405 (wireguard-2) |
| Osaka | `192.168.100.2` | 301, 302 | — |
| Kyoto | `192.168.100.3` | 100, 101, 200, 201 | 102 (apt-cache) |
| Rack-1 | `192.168.100.5` | 100, 101–105, 1000 (Fatman) | — |

Each guest's root account and existing `core` or `aloks98` account received the
Titan Air and Fatman public keys from `config/guest-ssh-keys.pub`. The retired
`aloksahoo@mac` key was removed by key material, including the same key labeled
`aloksahoo@acceldata.io` in the containers. All other existing key fingerprints
were preserved, including the node RSA keys and the distinct additional
`root@tokyo` key on wireguard-2.

Root SSH uses `PermitRootLogin prohibit-password` and public-key authentication.
Flatcar keys were installed in persistent `authorized_keys.d` fragments and the
active files regenerated with `update-ssh-keys`. Both new keys were installed
before independent SSH checks; the retired key was removed only after those
checks succeeded. Original authorization files and any previous policy drop-in
are retained under `/var/lib/pvectgen/ssh-access-backup-20261023` on each remaining
guest, accessible only to root. These backups contain the retired authorization
entries, so restoring them would restore that access.

Final verification passed for all 36 root/normal-user SSH sessions using Fatman's
key. Per-account fingerprint sets matched the originals minus the retired key
plus the two approved keys; active files, Flatcar fragments, effective root SSH
policy, and Flatcar's additional userdb key source were inspected. Titan Air's
public key was verified in every account; authentication from the Mac was not
tested. Boot IDs were unchanged between the installation and final login checks.
MOTDs and application configuration were preserved; no guest reboot was requested.

Two operational issues were resolved during verification. Newt-1 had a stale
known-host record; its public host key was checked through the trusted guest-agent
channel before updating the temporary verification record. Apt-cache's SSH daemon
failed to bind after a reload; its active SSH socket started a fresh daemon on
the next connection, and subsequent root login and policy checks passed.

At the user's subsequent request, Tokyo's `newt-1` VM 406 (`192.168.150.57`) was
shut down and destroyed with its three owned volumes (system, EFI, and cloud-init).
Its configuration and local-lvm volumes were confirmed absent. It had received
the key update before deletion and is excluded from the final counts above.
Hypervisor access keys and template disks were not changed in this rollout.

## Four-host minion enrollment, 2026-10-23

Tokyo (`192.168.100.1`), Osaka (`192.168.100.2`), Kyoto (`192.168.100.3`), and
Rack-1 (`192.168.100.5`) were upgraded from minion `0.3.2` to the tested project
binary `v0.3.2-33-g8ef4eb7`. Its embedded revision is
`8ef4eb7a93ffbc1cc53ee43179e290c8f2cceb1a`, with an unmodified source tree at build
time; subsequent repository changes were documentation only. Transferred binary
SHA-256 hashes were checked before installation and after replacement.

Existing minion configuration and API keys were retained. Each host received a
TLS identity and the current systemd unit, including `UMask=0077`. The minion
services were restarted individually and remain enabled at boot. Original
binaries, configuration directories, and service units are backed up under
`/root/pvectgen-minion-backup-20261023` on each host, accessible only to root.

All four nodes were enrolled by IP on port `50051` in Fatman's default manager
store, `/home/aloks98/.config/pvectgen/nodes.yaml`, with certificate pinning and
API-key authentication. Enrollment credentials were not added to the repository;
the node store, minion configuration, and TLS private keys have mode `0600`.

Readiness checks identified missing `snippets` content support on Tokyo, Osaka,
and Rack-1's existing `local` storage. It was enabled while preserving every
previous content type; original `storage.cfg` files are in the corresponding
host backups. Kyoto already supported snippets.

Final live checks passed for each node: authenticated health, all 18 doctor
checks, and VM inventory retrieval over pinned TLS. No build was submitted and
no guest was stopped or rebooted during enrollment.
