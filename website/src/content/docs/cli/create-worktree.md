---
title: Create worktree
description: Create and configure a Herdr worktree workspace with hwt.
---

`hwt create` turns a branch into a ready Herdr workspace.

```sh
hwt create --branch BRANCH [flags]
```

## What happens

1. hwt resolves the repository and merged global and project configuration.
2. Herdr creates the linked Git worktree, workspace, and root pane.
3. hwt copies, clones, or links configured files into the checkout.
4. Post-create commands run in order.
5. hwt records the base branch and returns the workspace details.

If file setup or a post-create command fails, hwt asks Herdr to remove the partially created worktree.

## Flags

| Flag                  | Description                                         |
| --------------------- | --------------------------------------------------- |
| `-b, --branch BRANCH` | Branch to create. Required.                         |
| `--base REF`          | Base ref. Defaults to the current branch.           |
| `--cwd PATH`          | Repository path. Defaults to the current directory. |
| `--path PATH`         | Override the configured worktree path.              |
| `--label LABEL`       | Herdr workspace label.                              |
| `--focus`             | Focus the new workspace.                            |
| `--json`              | Print machine-readable output.                      |

## Examples

Create from the current branch without changing focus:

```sh
hwt create --branch feat/agent-status
```

Create from `main`, focus the new workspace, and return structured output:

```sh
hwt create --branch feat/agent-status --base main --focus --json
```

The JSON result includes the workspace ID, root pane ID, checkout path, branch, base ref, configured agent, copied paths, and configuration sources.
