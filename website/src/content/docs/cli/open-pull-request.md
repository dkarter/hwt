---
title: Open a pull request
description: Resolve and open the GitHub pull request for a worktree branch.
---

`hwt url pr` resolves the pull request for the current worktree branch. HWT
provides this URL name by default for GitHub repositories:

To fetch code into a dedicated Herdr workspace and launch a review tool instead,
use [`hwt review`](/docs/cli/review-pull-request/).

```sh
hwt url pr
```

Pass a branch to resolve another pull request. The branch does not need to be
fetched locally because hwt queries GitHub through the authenticated `gh` CLI:

```sh
hwt url pr feature/name
```

## Machine-readable output

`--json` prints the name and resolved URL:

```sh
hwt url pr --json
```

```json
{
  "name": "pr",
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
hwt url pr contributor:feature/name --repo upstream/repository
```

The repository format is `[HOST/]OWNER/REPO`, matching `gh --repo`.

## Flags

| Flag                    | Description                                           |
| ----------------------- | ----------------------------------------------------- |
| `--cwd PATH`            | Repository path. Defaults to the current directory.   |
| `-R, --repo REPOSITORY` | GitHub base repository in `[HOST/]OWNER/REPO` format. |
| `--json`                | Print the resolved URL without opening a browser.     |
| `--open`                | Open the resolved HTTP(S) URL in the default browser. |

## Other forges

Override `urls.pr` for GitLab, Forgejo, or another forge. A template based on
`{branch}` does not require `gh`:

```yaml
urls:
  pr: https://gitlab.example/group/project/-/merge_requests?source_branch={branch}
```

The default template uses `{pr_owner}`, `{pr_repository}`, and `{pr_number}`.
HWT resolves those values with the authenticated GitHub CLI. Custom templates
that use any `pr_*` placeholder use the same lookup.

Without a branch argument, detached HEAD cannot identify a pull request; pass
the branch explicitly. Add `--open` to open the URL. On macOS hwt uses `open`;
on Linux it uses `xdg-open`.
