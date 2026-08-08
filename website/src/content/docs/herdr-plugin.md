---
title: Herdr plugin
description: Create and remove configured HWT worktrees through native Herdr actions.
---

:::caution[Experimental]
The Herdr plugin is experimental. Its installation, actions, and interaction
design may change between releases.
:::

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

Text inputs and the fuzzy base-branch picker support Vim-style normal and insert
modes. The interface shows the current mode and available controls.

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
