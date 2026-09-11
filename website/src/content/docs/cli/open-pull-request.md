---
title: Open a pull request
description: Resolve and open the GitHub pull request for a worktree branch.
---

`hwt pr` resolves the pull request for the current worktree branch and opens it
in the default browser:

To fetch code into a dedicated Herdr workspace and launch a review tool instead,
use [`hwt review`](/docs/cli/review-pull-request/).

```sh
hwt pr
```

Pass a branch to resolve another pull request. The branch does not need to be
fetched locally because hwt queries GitHub through the authenticated `gh` CLI:

```sh
hwt pr feature/name
```

## Machine-readable output

`--json` prints the resolved URL and does not open a browser:

```sh
hwt pr --json
```

```json
{
  "url": "https://github.com/owner/repository/pull/123"
}
```

## Remotes and forks

Pull request resolution currently supports GitHub and GitHub Enterprise through
`gh`. Authenticate `gh` for the repository's host before running the command.

When a repository has multiple remotes, select the pull request's base
repository explicitly. This also supports fork pull requests; qualify the head
branch with its owner if GitHub has more than one matching branch:

```sh
hwt pr contributor:feature/name --repo upstream/repository
```

The repository format is `[HOST/]OWNER/REPO`, matching `gh --repo`.

## Flags

| Flag                    | Description                                           |
| ----------------------- | ----------------------------------------------------- |
| `--cwd PATH`            | Repository path. Defaults to the current directory.   |
| `-R, --repo REPOSITORY` | GitHub base repository in `[HOST/]OWNER/REPO` format. |
| `--json`                | Print the resolved URL without opening a browser.     |

Without a branch argument, detached HEAD cannot identify a pull request; pass
the branch explicitly. On macOS hwt opens URLs with `open`; on Linux it uses
`xdg-open`.
