---
title: Check out a branch or pull request
description: Open an existing branch or PR in a configured Herdr worktree without starting a reviewer.
---

`hwt checkout` accepts a GitHub pull request URL, number, local branch, or remote branch:

```sh
hwt checkout origin/feature/name
hwt checkout feature/name
hwt checkout 123
hwt checkout https://github.com/owner/repository/pull/123
```

The worktree uses the original branch name. Remote-qualified names fetch from
that remote; an unfetched plain name prefers `origin`. PRs use GitHub's pull
request head ref, including for forks, and verify the fetched SHA against metadata.
The primary checkout is never switched or reset.

New worktrees go through the same lifecycle as `hwt create`: configured paths,
file copies, environment and port allocation, local DNS, and `post_create` hooks.
Unlike [`hwt review`](../review-pull-request/), checkout never starts a reviewer,
even if `review_command` uses `{pr_url}` or its executable is not installed.

## Reuse

```sh
hwt checkout origin/feature/name --reuse
```

`--reuse` opens an existing linked worktree, or puts an existing local branch in
a new worktree. Existing local commits and uncommitted work are preserved rather
than reset to the fetched remote tip. Opening an existing worktree does not repeat
file copies or creation hooks. Closed workspaces are reopened.

Without `--reuse`, a local branch at a different commit is rejected. An existing
worktree requires `--reuse` or `--focus`. A branch checked out in the primary
checkout cannot be opened as another worktree.

## Flags and JSON

| Flag                    | Description                                                                                   |
| ----------------------- | --------------------------------------------------------------------------------------------- |
| `--cwd PATH`            | Repository path; defaults to the current directory.                                           |
| `--remote NAME`         | Select a Git remote.                                                                          |
| `-R, --repo REPOSITORY` | GitHub repository for PR selectors.                                                           |
| `--reuse`               | Preserve and open an existing branch or worktree.                                             |
| `--focus`               | Focus the workspace.                                                                          |
| `--path PATH`           | Override the configured worktree path.                                                        |
| `--label TEXT`          | Set the Herdr workspace label.                                                                |
| `--json`                | Print target identity, actual commit, branch, path, workspace and pane IDs, and reuse status. |

The JSON `review_command.status` is `not_requested`.
