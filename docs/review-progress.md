# Architecture review: items 1–14

The manager owns editable definitions and a local build-history projection. The minion owns execution, Proxmox resource checks, process lifetimes and durable event journals. CLI and TUI use the same build coordinator. Custom build steps remain trusted administrator shell commands.

| Item | Change | Verification |
| --- | --- | --- |
| 1 | Go/dependency updates and CI vulnerability gate (`8397357`) | Unit/race tests, cross-builds, vulnerability scan |
| 2 | Validate source/destination IDs and resource types; require explicit overwrite | Fake-runner regression tests and live refusal/replacement tests |
| 3 | TLS 1.3, enrollment certificate pinning, serialized identity provisioning, private credential files | TLS handshake tests, concurrent provisioning, live authenticated RPCs |
| 4 | Atomic downloads, mandatory SHA-256/SHA-512, verified cache reuse | Corruption, cancellation, HTTP failure and manifest-format tests |
| 5 | Unique workspaces/snippets and cancellable VM/cache locks | Concurrent same-name builds, lock contention, cleanup tests |
| 6 | Atomic YAML transactions with process locks and propagated read/write errors | Concurrent store handles, corruption preservation, missing-record tests |
| 7 | Strict config decoding, bounded IDs/counts, path/network/init validation and quoted substitutions | Invalid input, traversal, YAML, Ignition and shell interpolation tests |
| 8 | Shared CLI/TUI build coordinator and event projection | Terminal-event requirement and idempotent replay tests |
| 9 | Durable minion jobs, reconnect/watch, real cancellation and child cleanup | Restart/torn-journal tests and live disconnect/cancel/replay |
| 10 | Failure exit codes, default node, VM list, hostname/DNS/profile launch flags | Live CLI failures and successful guest boot |
| 11 | Verified atomic installers/updater and embedded defaults | Offline installer rejection/preservation test, archive tests, import outside checkout |
| 12 | Remove shared passwords, fixed hostnames and personal keys; site settings in profiles; Fedora DNF5 fixes | Bundled config resolution; Debian cloud-init boot test |
| 13 | Node/template profiles for storage, snippets, bridge, disk format/size and guest settings | Live build with dedicated snippet storage and 8 GiB disk |
| 14 | `manager doctor`, automatic build/launch preflight, fleet dry-run checks | Live valid profile and missing-storage rejection |

The approved TUI direction uses Workspace navigation and a guided build flow.
Implementation steps, committed separately:

| Step | Change | Verification |
| --- | --- | --- |
| 15 | Responsive workspace shell, persistent sidebar and compact Ctrl+G navigation | Terminal-size, form-focus and background-build tests |
| 16 | Filterable template browser, selected details, profile editing, delete confirmation, adaptive tables/forms | Unicode/paging, filtered actions and focused-field visibility tests |
| 17 | Guided build review with validated snapshots and live preflight gate | Failed/stale checks, invalid IDs, immutable requests and duplicate-submit tests |
| 18 | Persistent progress logs, paused follow, explicit reconnect and remote cancellation | Race tests for replay and queue cancellation; live terminal checks |
| 19 | Alacritty Command+G mapping instructions and explicit Mac shortcut hints | Navigation toggle, form focus, removed F2 alias and CLI tests |
| 20 | Artboard palette, padded shell, filled selections, balanced columns, consistent headings and template detail grid | Actual render inspection, long-list/wide-terminal regressions, TUI/CLI race tests and remote PTY smoke test |

The implementation uses the existing CLI build coordinator and minion jobs. See
[TUI controls and limitations](tui.md). Artboard sources remain uncommitted in the
separate prototype worktree.

See [migration](migration.md) before upgrading an existing installation and [testing](testing.md) for the test record and limits.
