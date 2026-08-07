# HWT Worktrees for Herdr

This plugin adds Herdr actions for interactively creating and safely removing
configured HWT worktrees.

HWT must be installed and available in the Herdr server's `PATH`.

```sh
herdr plugin install dkarter/hwt/plugins/herdr
```

For local development from the HWT checkout:

```sh
mise run build
herdr plugin link ./plugins/herdr
```

When linked from the repository, the plugin automatically uses the local
`./hwt` development binary if it exists.

Run the automated lifecycle test from a Herdr-managed pane:

```sh
./scripts/test-herdr-plugin-e2e.sh
```

The test uses controllable tabs in a disposable background workspace instead
of the normal modal popup, then verifies creation, configuration, unchanged
user focus, dirty-worktree confirmation, removal, and branch preservation.

The plugin provides these qualified actions:

- `hwt.worktrees.new`
- `hwt.worktrees.remove`

Text inputs use Vim-style modes. They start in insert mode with a bar cursor;
press `Esc` for normal mode and a block cursor. Normal mode supports `h`, `l`,
`0`, `$`, `w`, `b`, `e`, `B`, `E`, `x`, `s`, `i`, `a`, `I`, and `A`. Press
`d` followed by a motion to delete its range; `dd` clears the input. The
base-branch picker filters fuzzily and adds `j`/`k`, arrow keys, `g`, and `G`.
Confirmation dialogs use `h`/`l` or the arrow keys to choose between Yes and
No, with No selected by default.

Bind them in `~/.config/herdr/config.toml` if desired:

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
