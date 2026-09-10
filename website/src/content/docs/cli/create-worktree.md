---
title: Create worktree
description: Create and configure a Herdr worktree workspace with hwt.
---

`hwt create` can create a ticket from a task description and turn its returned branch into a ready Herdr workspace. Explicit branch creation remains available through `--branch`.

```sh
hwt create DESCRIPTION [flags]
hwt create --branch BRANCH [flags]
```

These forms are mutually exclusive. A positional argument is always a task description, never a branch name.

## What happens

1. hwt resolves the repository, base ref, and merged global and project configuration.
2. With a description, hwt runs the configured ticket command and reads `branchName` from its JSON output.
3. Herdr creates the linked Git worktree, workspace, and root pane using that branch unchanged.
4. hwt copies, clones, or links configured files into the checkout.
5. hwt reserves configured ports and writes the ignored `.env.worktree` file.
6. Post-create commands run in order with the generated environment.
7. hwt records the base branch and returns the workspace details.

If file setup or a post-create command fails, hwt asks Herdr to remove the partially created worktree.

## Flags

| Flag                  | Description                                         |
| --------------------- | --------------------------------------------------- |
| `-b, --branch BRANCH` | Explicit branch to create instead of a ticket.      |
| `--base REF`          | Base ref. Defaults to the current branch.           |
| `--cwd PATH`          | Repository path. Defaults to the current directory. |
| `--path PATH`         | Override the configured worktree path.              |
| `--label LABEL`       | Herdr workspace label.                              |
| `--focus`             | Focus the new workspace.                            |
| `--json`              | Print machine-readable output.                      |

## Examples

Create a Linear ticket with the default `lnr quick --json` integration, then create its worktree:

```sh
hwt create 'Add agent status to the dashboard'
```

Create from the current branch without changing focus:

```sh
hwt create --branch feat/agent-status
```

Create from `main`, focus the new workspace, and return structured output:

```sh
hwt create --branch feat/agent-status --base main --focus --json
```

The JSON result includes the workspace ID, root pane ID, checkout path, branch, base ref, configured agent, copied paths, generated environment, and configuration sources.

The task description is appended to `ticket_command` as one argument without invoking a shell. The command must print one JSON object with a non-empty string `branchName`. Command failures include the exit status and stderr. Invalid output and detectable branch or checkout-path conflicts stop before Herdr creates a worktree; the remote ticket may already exist when a conflict can only be known from its returned branch.
