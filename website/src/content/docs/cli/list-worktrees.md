---
title: List worktrees
description: List the Herdr worktrees associated with a Git repository.
---

`hwt list` returns Herdr's machine-readable worktree inventory for a repository.

```sh
hwt list [--cwd PATH]
```

By default, hwt resolves the repository from the current directory and invokes Herdr's worktree list command with JSON output.

## Flags

| Flag         | Description                                         |
| ------------ | --------------------------------------------------- |
| `--cwd PATH` | Repository path. Defaults to the current directory. |

## Examples

List worktrees for the current repository:

```sh
hwt list
```

List worktrees without changing directories:

```sh
hwt list --cwd ~/code/project
```
