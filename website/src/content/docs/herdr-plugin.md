---
title: Herdr plugin
description: Create and remove configured HWT worktrees through native Herdr actions.
---

The official HWT plugin adds interactive worktree actions to Herdr. It does not
require a separate fuzzy finder or command palette.

HWT must be installed and available in the Herdr server's `PATH`.

## Install

Install the plugin directly from this repository:

```sh
herdr plugin install dkarter/hwt/plugins/herdr
```

The plugin provides these actions:

- **New configured worktree** prompts for a branch name and base branch, then
  creates and focuses the configured worktree.
- **Remove current worktree** confirms the checkout path and uses HWT's safe,
  fast removal. Dirty or locked worktrees require a separate force confirmation.

## Controls

Text inputs start in insert mode with a bar cursor. Press `Esc` to enter normal
mode and switch to a block cursor.

Normal-mode input controls:

- `h` and `l` move by one character.
- `0` and `$` move to the beginning or end.
- `w`, `b`, and `e` move by Vim word boundaries.
- `B` and `E` move by whitespace-delimited WORD boundaries.
- `x` deletes the character under the cursor.
- `s` substitutes the character under the cursor and enters insert mode.
- `d` followed by a motion deletes that motion's range; `dd` clears the input.
- `i`, `a`, `I`, and `A` enter insert mode in the corresponding Vim position.
- `q` cancels and Enter submits.

The base-branch picker filters branches fuzzily as you type. In normal mode,
use `j` and `k` to move through matches and `g` or `G` to jump to the first or
last match. Arrow keys and `Ctrl+N`/`Ctrl+P` also navigate while inserting.

Confirmation dialogs show horizontal Yes and No choices. Use `h`/`l`, the
arrow keys, or Tab to change the selection, then press Enter. `y` and `n`
submit directly. Destructive confirmations select No by default.

## Keybindings

Plugin actions do not replace your existing keybindings. Add bindings to
`~/.config/herdr/config.toml` if desired:

```toml
[[keys.command]]
key = "prefix+shift+g"
type = "plugin_action"
command = "hwt.worktrees.new"
description = "new configured worktree"

[[keys.command]]
key = "prefix+shift+d"
type = "plugin_action"
command = "hwt.worktrees.remove"
description = "remove configured worktree"
```

Reload Herdr configuration after editing it:

```sh
herdr server reload-config
```

## Local development

Link the plugin from an HWT checkout while developing it:

```sh
mise run build
herdr plugin link ./plugins/herdr
```

The linked plugin automatically uses the checkout's `./hwt` development binary
when it exists.

Run the end-to-end lifecycle test from a Herdr-managed pane:

```sh
./scripts/test-herdr-plugin-e2e.sh
```

The script creates a disposable repository and runs controllable plugin tabs in
a background workspace instead of using the normal modal popup. It verifies
that focus never leaves your current workspace, along with branch-name normalization,
base-branch selection, configured file copying, post-create hooks, workspace
creation, dirty-worktree force confirmation, checkout removal, workspace
closure, and branch preservation. It removes its temporary repository when
finished.

The popup workflows can also be run directly:

```sh
hwt herdr create
hwt herdr remove
```
