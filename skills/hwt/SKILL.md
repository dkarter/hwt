---
name: hwt
description: Create, inspect, and remove Herdr-managed Git worktrees with hwt. Use when an agent needs an isolated Herdr workspace, must clean one up safely, or is asked to create or edit a project's .herdr-worktree.yaml configuration.
---

# hwt

Use `hwt` for the lifecycle of Herdr-managed worktrees. Do not mix it with
`git worktree remove`, workmux, or another lifecycle manager.

## Create

Choose a valid new branch name and an explicit base ref, then run:

```bash
hwt create --cwd <repository-path> --branch <branch> --base <base-ref> --json
```

Creation is unfocused by default. Parse `workspace_id`, `pane_id`, `path`, and
`agent` from the JSON response; never derive IDs or predict the configured path.
Use the returned IDs for subsequent Herdr commands.

Omit `--base` only when intentionally using the current branch. A detached HEAD
requires an explicit base. Use `--focus` only when the user asks to switch to the
new workspace.

## Inspect

```bash
hwt list --cwd <repository-path>
```

Use this response to discover worktree paths and workspace IDs. Do not infer
ownership from directory names.

## Remove

```bash
hwt remove --workspace <workspace-id> --json
```

Removal closes the workspace and removes the linked checkout, but does not
delete its Git branch. If removing the current workspace, invoke removal from a
surviving pane or workspace because the caller will be terminated.

Use `--force` only after reporting the dirty or locked state and receiving
explicit approval to discard it. The flag deletes uncommitted and untracked
files.

## Project Configuration

When asked to create, edit, or explain `.herdr-worktree.yaml`, load the bundled
reference with `hwt skill config`. Do not load it for ordinary create, list, or
remove operations.

If syntax may differ from this skill, treat the installed CLI as authoritative:

```bash
hwt --help
hwt <command> --help
```
