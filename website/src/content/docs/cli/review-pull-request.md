---
title: Review a pull request
description: Fetch a pull request or branch into an exact, reusable Herdr review workspace.
---

`hwt review` accepts one full GitHub pull request URL or branch reference:

```sh
hwt review https://github.com/owner/repository/pull/123
hwt review origin/feature/name
```

The command never checks out or resets the primary checkout. It creates a
deterministic local branch below `hwt/review/`, creates the worktree through the
normal HWT lifecycle, opens its Herdr workspace without taking focus, and starts
the configured review command. Pass `--focus` to switch to the review workspace.

## Pull requests and forks

Pull request URLs must use HTTPS and exactly
`https://HOST/OWNER/REPO/pull/NUMBER`, without a query, fragment, port, or
userinfo. `github.com` is supported directly. A GitHub Enterprise host must
match a configured Git remote. HWT queries metadata with authenticated `gh`,
fetches the base repository's `refs/pull/NUMBER/head` and base branch into
private `refs/hwt/reviews/` refs, and verifies that the fetched head SHA matches
GitHub's response. This synthetic head ref supports same-repository and fork pull
requests while it remains available on the server.

With multiple remotes, HWT matches the URL's host, owner, and repository. Use
`--remote NAME` if that is not unique. `--repo [HOST/]OWNER/REPO` is passed to
`gh` and must identify the repository in the URL.

## Branch references

A local branch is treated as already fetched. `REMOTE/BRANCH` fetches that
branch without changing a checkout. An unfetched plain branch works when there
is exactly one remote; with multiple remotes, use `REMOTE/BRANCH` or
`--remote NAME`. Detached primary HEAD is rejected for branch selectors because
there is no unambiguous review base. Pull request URLs carry their base metadata
and still work from detached HEAD.

## Exact reuse

HWT reuses a linked review worktree only when its deterministic review branch,
HEAD commit, and pinned base commit all match the request. A mismatch is an error; HWT
never resets or silently attaches to another commit. If the worktree is closed,
HWT reopens it and launches the reviewer. If its Herdr workspace is already
open and its recorded review pane is busy, HWT returns it with
`review_command.status` set to `already_open` and does not start a duplicate
process. If that pane is idle because the tool exited, HWT launches it again.

Generated worktree paths continue to use `worktree_dir`, `worktree_naming`, and
`worktree_prefix`. New review worktrees receive the normal copied files,
environment/port allocation, local DNS registration, and `post_create` hooks.

## Review command and recovery

`review_command` is an argv array and defaults to `[tuicr]`. HWT adds no remote
metadata to it and shell-quotes each configured argument before Herdr runs it in
the checkout. Missing executables and Herdr launch failures return an error after
reporting the successful workspace in JSON. The workspace remains intact so the
tool can be installed or started manually. `launched` means Herdr accepted the
command; later early exits or non-zero statuses do not remove the workspace.

## JSON output

```sh
hwt review --json https://github.com/owner/repository/pull/123
```

The result includes PR or branch identity, the verified commit and local review
branch, checkout path, workspace and pane IDs, `reused`, and the review command's
argv, status, and optional error. On a launch failure HWT prints this recoverable
result before returning a non-zero status.

## Flags

| Flag                    | Description                                                         |
| ----------------------- | ------------------------------------------------------------------- |
| `--cwd PATH`            | Repository path. Defaults to the current directory.                 |
| `--remote NAME`         | Remote to fetch when selection is ambiguous.                        |
| `-R, --repo REPOSITORY` | GitHub repository for a PR URL in `[HOST/]OWNER/REPO` form.         |
| `--focus`               | Focus the created or reused workspace.                              |
| `--json`                | Print identity, checkout, workspace, reuse, and launch information. |
