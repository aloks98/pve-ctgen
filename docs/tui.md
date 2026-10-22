# Workspace TUI

Start with `pvectgen manager tui` (or add `--config /path/to/manager.yaml`).
The initial screen is Templates. At 100 columns or wider, the workspace sidebar
stays visible. In narrower terminals, Ctrl+G opens the same navigation as a menu.
80 × 24 is recommended; the compact layout supports at least 40 × 12.
The workspace uses the artboard's canvas and selection colors, with light/dark
variants, padded content, and a maximum width of 156 cells for readability.
Template details appear in two columns when there is enough vertical space;
short terminals prioritize the list. Tables distribute spare width across columns.

The exact palette requires true color. If SSH loses the color capability hint,
Alacritty users can launch with `COLORTERM=truecolor pvectgen manager tui`.
When using tmux, its outer terminal must also be configured for RGB color;
otherwise tmux may reduce the palette to 256 colors.

## Command+G on a Mac using Alacritty

Add this binding to `~/.config/alacritty/alacritty.toml` **on your Mac**:

```toml
[[keyboard.bindings]]
key = "G"
mods = "Command"
chars = "\u0007"
```

Command+G now sends the same control byte as Ctrl+G through SSH and tmux.
Ctrl+G remains available on all keyboards. If you already bind Command+G, replace
that binding instead of adding a second one. The TUI uses the alternate screen;
you can add `mode = "Alt"` to limit the binding to alternate-screen applications
(tmux itself also uses that screen).

Start `pvectgen manager tui --mac-keys` on the VM to show `cmd+g` in the footer.
This flag changes hints only; the Alacritty binding enables the shortcut.
SSH does not reliably identify the client keyboard, so the VM does not guess
from its own operating system. A tmux binding for Ctrl+G would intercept it;
leave that key unbound in tmux's root key table.

See the [Alacritty keyboard binding reference](https://alacritty.org/config-alacritty.html#keyboard).

## Navigation and definitions

- Ctrl+G focuses the workspace menu; use j/k or arrows, then Enter. Number keys 1–9
  select destinations while the menu has focus. Ctrl+G or Escape closes the menu.
- Forms and confirmations retain keyboard focus. Escape returns to the previous
  stage; from a top-level destination it returns to Templates.
- Templates: `/` filters by name, tag, profile or init type. Enter/Escape leaves
  the filter field while retaining the query. Clear the field to show all items.
- `b` opens a build for the selected template. `a` adds, `e` edits, `d` asks for
  deletion confirmation, and Enter opens scrollable details. Template deletion
  removes the local definition only.
- Tables support j/k, arrows, Home/End, g/G and Page Up/Page Down. Forms use Tab,
  Shift+Tab or arrows, and scroll to keep the focused field visible.

## Reviewed builds

The flow is Select → Node → VM IDs → Review. Starting from a template skips the
selection stage. Multi-selection remains available through New build.

Review resolves initialization content and build steps into a validated snapshot,
then runs read-only preflight checks on the selected node. It displays the target
IDs, profiles and check results. Use j/k to scroll through the checks. Failed or
empty results block submission; `r` retries. Enter starts only after successful
checks. The submitted request is the snapshot that was reviewed. The node checks
resources again during execution, so preflight is not a reservation.

The TUI refuses to replace an existing VM/template. Use a free target ID or the
CLI's explicit overwrite option when replacement is intended.

## Progress and recovery

Wide layouts show steps next to logs; narrow layouts prioritize the current step
and log. Completed logs remain visible. Scrolling upward pauses automatic log
follow; `f` or End resumes it. The live buffer retains 5,000 lines; the coordinator
persists the full build history.

Escape backgrounds the build. Open Build progress from the workspace to return.
Only one queue runs in a TUI session. On a stream interruption, the queue pauses;
`r` watches the existing job from its persisted event cursor without resubmitting
it. `x` opens cancellation confirmation. Confirming with `y` calls the minion's
cancel API and prevents subsequent queued builds from starting. Cancellation
errors are displayed and can be retried.

`q` quits from Templates; Ctrl+C quits from any screen. Quitting disconnects the
manager and stops its unstarted queue; it does not cancel an accepted minion job.
After restarting, use History to find its ID and
`pvectgen manager build watch BUILD_ID` to resume observation. In-session reconnect
is supported in the TUI; restoring a previous session's queue is not implemented.
