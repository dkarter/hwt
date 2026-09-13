---
title: Create worktree
description: Create and configure a Herdr worktree workspace with hwt.
---

`hwt create` creates a ready Herdr workspace from a literal branch or from a
configured ticket command.

```sh
hwt create BRANCH [flags]
hwt create --ticket [INPUT] [flags]
hwt create --ticket=NAME [INPUT] [flags]
hwt create --branch BRANCH [flags]
```

A positional value is a literal branch name unless `--ticket` is present.
`--ticket` selects `ticket_commands.default`; `--ticket=NAME` selects another
named command. Ticket input is optional so commands may provide an interactive
picker.

## What happens

1. hwt resolves the repository, base ref, and merged global and project configuration.
2. With `--ticket`, hwt substitutes the optional input into the selected command's argv and runs it.
3. hwt reads the branch and ticket metadata through the command's output selectors.
4. Herdr creates the linked Git worktree, workspace, and root pane using that branch unchanged.
5. hwt copies, clones, or links configured files into the checkout.
6. hwt reserves configured ports and writes the ignored `.env.worktree` file.
7. Post-create commands run in order with the generated environment.
8. hwt records the base branch and returns the workspace details.

If file setup or a post-create command fails, hwt asks Herdr to remove the partially created worktree.

## Flags

| Flag                  | Description                                                   |
| --------------------- | ------------------------------------------------------------- |
| `-b, --branch BRANCH` | Explicit branch to create.                                    |
| `--ticket[=NAME]`     | Run the default or named ticket command to derive the branch. |
| `--base REF`          | Base ref. Defaults to the current branch.                     |
| `--cwd PATH`          | Repository path. Defaults to the current directory.           |
| `--path PATH`         | Override the configured worktree path.                        |
| `--label LABEL`       | Herdr workspace label.                                        |
| `--focus`             | Focus the new workspace.                                      |
| `--json`              | Print machine-readable output.                                |

## Examples

Create a literal branch without touching an issue tracker:

```sh
hwt create investigation-cache
```

Select an existing Linear issue with the configured default picker:

```sh
hwt create --ticket
hwt create --ticket 'authentication bug'
```

Create a new Linear issue with a named command:

```sh
hwt create --ticket=create 'Add agent status to the dashboard'
```

Create from `main`, focus the new workspace, and return structured output:

```sh
hwt create --branch feat/agent-status --base main --focus --json
```

The JSON result includes the workspace ID, root pane ID, checkout path, branch, base ref, configured agent, copied paths, generated environment, and configuration sources.

HWT replaces `{input}` directly in each configured argument without invoking a
shell. If no input is supplied, an argument that is exactly `{input}` is
omitted, which allows an interactive command to open its picker. An embedded
placeholder such as `--query={input}` requires input. Commands without an input
placeholder receive no additional argument. Command stdout is reserved for the
final JSON object, so interactive picker UI must use stderr.

The selected command must print one JSON object. Its output configuration maps
a branch selector and optional metadata selectors to string values. Selectors
may address nested object fields with dot notation. Command failures include
the exit status and stderr. Invalid output and detectable branch or checkout
path conflicts stop before Herdr creates a worktree.
