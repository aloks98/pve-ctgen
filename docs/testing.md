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

Only Debian guest boot was exercised end to end. Other bundled cloud-init files and Flatcar Butane were imported/resolved locally; their full guest provisioning, Kubernetes extension downloads, alternate storage backends and multi-node fleet creation were not live-tested.

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
