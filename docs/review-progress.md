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

The full TUI redesign remains the next phase (items 15–18). This phase shares its build execution and persistence with the CLI.

See [migration](migration.md) before upgrading an existing installation and [testing](testing.md) for the test record and limits.
