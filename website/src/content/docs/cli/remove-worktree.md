---
title: Remove worktree
description: Safely remove Herdr worktrees without waiting for large dependency trees.
---

`hwt remove` removes a linked worktree and closes its Herdr workspace. `hwt rm` is an alias.

```sh
hwt remove [flags]
```

Run it from the workspace being removed, or provide a workspace ID from anywhere:

```sh
hwt remove --workspace w_8Jx2
```

## Why removal is fast

Deleting a large worktree synchronously can take a long time when directories such as `node_modules`, `deps`, or build caches contain hundreds of thousands of entries. The terminal remains blocked while the filesystem walks and deletes every file.

hwt keeps that wait out of the critical path:

1. It validates that the workspace is a linked Herdr worktree and checks its Git metadata.
2. Unless `--force` is set, it refuses locked worktrees and checkouts with uncommitted or untracked files.
3. It atomically renames the checkout into a temporary sibling directory on the same filesystem.
4. It closes the Herdr workspace and removes the small Git worktree metadata entry.
5. It starts a detached background deletion for the renamed directory and returns without waiting for the recursive delete.

Under normal conditions, the expensive recursive deletion still happens without blocking the person or agent issuing the command. If the background process cannot start, hwt falls back to finishing the cleanup synchronously. If closing the workspace or removing metadata fails, hwt attempts to move the checkout back into place.

This fast remove path is a key difference from waiting for Herdr's built-in worktree removal to synchronously delete a dependency-heavy checkout.

## Flags

| Flag                 | Description                                      |
| -------------------- | ------------------------------------------------ |
| `-w, --workspace ID` | Workspace ID. Defaults to the current workspace. |
| `-f, --force`        | Remove a dirty or locked worktree.               |
| `--json`             | Print machine-readable output.                   |

:::caution
`--force` bypasses the dirty and locked checks. It can permanently remove uncommitted and untracked files.
:::
