---
name: hwt
description: Create, review, inspect, and remove Herdr-managed Git worktrees with hwt. Use when an agent needs an isolated Herdr workspace, a pull request review checkout, safe cleanup, or project configuration.
---

# hwt

Use `hwt` for the lifecycle of Herdr-managed worktrees. Do not mix it with
`git worktree remove`, workmux, or another lifecycle manager.

## Create

To create a ticket and worktree together, pass the task description as one argument:

```bash
hwt create --cwd <repository-path> --base <base-ref> --json '<description>'
```

For explicit branch creation, choose a valid new branch name and run:

```bash
hwt create --cwd <repository-path> --branch <branch> --base <base-ref> --json
```

A positional argument is always a ticket description. Never combine it with `--branch`.

Creation is unfocused by default. Parse `workspace_id`, `pane_id`, `path`, and
`agent` from the JSON response; never derive IDs or predict the configured path.
Use the returned IDs for subsequent Herdr commands.

Omit `--base` only when intentionally using the current branch. A detached HEAD
requires an explicit base. Use `--focus` only when the user asks to switch to the
new workspace.

## Review

Create or reuse an exact-commit review workspace from a full GitHub pull request
URL or branch reference:

```bash
hwt review --cwd <repository-path> --json <pull-request-url-or-branch>
```

The command does not focus the workspace unless `--focus` is passed. Parse
`commit`, `path`, `workspace_id`, `pane_id`, `reused`, and `review_command.status`
from JSON instead of predicting them. Use `--remote NAME` when a branch or pull
request base remote is ambiguous. An `already_open` launch status means HWT
reused the existing review session and deliberately did not launch a duplicate
tool because its recorded review pane is still busy. An idle pane is relaunched.
A failed review tool launch leaves the checkout and workspace available for
repair or manual launch.

## Inspect

```bash
hwt list --cwd <repository-path>
```

Use this response to discover worktree paths and workspace IDs. Do not infer
ownership from directory names.

Resolve any configured URL with `hwt url <name> [branch]`. Plain and `--json`
output never open a browser; use `--open` only when explicitly requested. Never
open database or other non-HTTP(S) URLs. `hwt url pr [branch]` uses the default
GitHub URL unless configuration replaces it. If multiple remotes exist, pass
the GitHub base repository with `--repo OWNER/REPO`. Resolve a configured
preview with `hwt url preview [branch]`. An explicit branch cannot use a
template containing `{worktree}`. Use `hwt url --json` to resolve every
configured URL.

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
