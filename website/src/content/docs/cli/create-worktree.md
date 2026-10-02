---
title: Create worktree
description: Create and configure a Herdr worktree workspace with hwt.
---

`hwt create` creates a ready Herdr workspace from a free-form title, an explicit
branch, or a configured ticket command.

```sh
hwt create TITLE [flags]
hwt create --ticket [INPUT] [flags]
hwt create --ticket=NAME [INPUT] [flags]
hwt create --branch BRANCH [flags]
```

A positional value is a free-form title whose whitespace is normalized to
hyphens unless `--ticket` is present. Use `--branch` when the branch name must
be passed unchanged. `--ticket` selects `ticket_commands.default`;
`--ticket=NAME` selects another named command. Ticket input is optional so
commands may provide an interactive picker.

## What happens

1. hwt resolves the repository, base ref, and merged global and project configuration.
2. With `--ticket`, hwt substitutes the optional input into the selected command's argv and runs it.
3. hwt reads the branch and ticket metadata through the command's output selectors.
4. Herdr creates the linked Git worktree, workspace, and root pane using the resulting branch.
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
| `--focus`             | Focus the workspace; implies reuse of an existing checkout.   |
| `--reuse`             | Reuse an existing branch checkout without stealing focus.     |
| `--json`              | Print machine-readable output.                                |

## Examples

Create a normalized branch without touching an issue tracker:

```sh
hwt create 'investigation cache'
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

## Existing and stale checkouts

For explicit branches and normalized titles, `--reuse` or `--focus` returns an
existing linked checkout. HWT reuses its open Herdr workspace or opens one at the
existing path. Without either flag, HWT reports the conflicting checkout path.
Ticket-generated branches retain their existing conflict policy and are not reused.

Reuse does not copy files again, run `post_create`, reset the branch, or rewrite
ticket or base metadata. Ports remain allocated with the same values, even if port
configuration changed; run `hwt env` explicitly to reconcile those changes. The returned
`base` is the recorded base, or an empty string if none was recorded; `--base` is
not applied to a reused checkout. Configured placement affects new checkouts only.
An explicit `--path` must match the existing checkout. Primary checkouts, ambiguous
records, and invalid checkouts are never reused.

JSON always includes `reused_worktree` and `reused_workspace`:

| Outcome                                   | `reused_worktree` | `reused_workspace` |
| ----------------------------------------- | ----------------- | ------------------ |
| New checkout and workspace                | `false`           | `false`            |
| Existing checkout, newly opened workspace | `true`            | `false`            |
| Existing checkout and open workspace      | `true`            | `true`             |

A missing or prunable checkout returns a nonzero exit status without deleting
anything. With `--json`, the diagnostic includes `error: "stale_worktree"`,
`branch`, and `path`. For a confirmed missing, unlocked linked checkout, `repair`
contains a targeted command as an argv array. Review it, run it, then retry:

```sh
git -C /path/to/repo worktree remove --force -- '/path/to/missing checkout'
hwt create --cwd /path/to/repo --branch feature/example --base main --focus --json
```

HWT never runs broad `git worktree prune`. Locked records or prunable entries whose
directories still exist require manual inspection; no removal command is suggested.

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
