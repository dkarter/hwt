---
title: Worktree URLs
description: Configure, resolve, and open named URLs for worktrees and branches.
---

HWT maps names such as `pr`, `preview`, `ticket`, and `local` to URLs. A URL can
use a template or expose the generated URL for a configured local service.

```yaml
urls:
  preview:
    template: https://{sanitized_branch}.preview.example.com
    label: Branch preview
  ticket: https://linear.example/issue/{ticket.identifier}
  local:
    service: web
    label: Local app
```

Global and repository URL maps merge by name, with repository entries winning.
A repository entry replaces the complete global entry, including its label.

## Resolve URLs

Resolve a named URL for the current worktree:

```sh
hwt url preview
```

Pass a branch to resolve a URL for a branch that is not checked out locally:

```sh
hwt url preview feature/name
```

An explicit branch has no associated worktree, so its template cannot use
`{worktree}`, `{hostname}`, worktree-local `ticket.*` metadata, or a service URL.

Use `--open` to open an HTTP(S) URL in the default browser. HWT never opens URLs
by default, and `--open` rejects other schemes. HWT only resolves a preview URL;
it does not poll a deployment provider for readiness.

## Pull requests

HWT provides `pr` by default for GitHub repositories:

```sh
hwt url pr
hwt url pr feature/name
```

HWT resolves pull request details through the authenticated `gh` CLI. With
multiple remotes, select the base repository explicitly. Qualify the head branch
with its owner when GitHub has more than one match:

```sh
hwt url pr contributor:feature/name --repo upstream/repository
```

The repository format is `[HOST/]OWNER/REPO`, matching `gh --repo`. Override
`urls.pr` for another forge or for a branch-based URL that does not require `gh`:

```yaml
urls:
  pr: https://gitlab.example/group/project/-/merge_requests?source_branch={branch}
```

To fetch code into a dedicated Herdr workspace and launch a review tool instead,
use [`hwt review`](./cli/review-pull-request/).

## Placeholders

| Placeholder          | Value                                                                   |
| -------------------- | ----------------------------------------------------------------------- |
| `{repository}`       | Primary checkout directory name.                                        |
| `{branch}`           | Current branch, or the explicit branch argument.                        |
| `{sanitized_branch}` | Branch normalized for hostnames and identifiers.                        |
| `{worktree}`         | Current checkout directory name; unavailable with an explicit branch.   |
| `{hostname}`         | Stable HWT local hostname; unavailable with an explicit branch.         |
| `{pr_host}`          | GitHub pull request host resolved lazily with authenticated `gh`.       |
| `{pr_owner}`         | GitHub pull request owner resolved lazily with authenticated `gh`.      |
| `{pr_repository}`    | GitHub pull request repository resolved lazily with authenticated `gh`. |
| `{pr_number}`        | GitHub pull request number resolved lazily with authenticated `gh`.     |

[Worktree metadata](../worktree-metadata/) adds static, ticket, and lazy command
placeholders such as `{ticket.identifier}` and `{database.host}`.

Sanitization lowercases ASCII letters, replaces each run outside `a-z` and `0-9`
with one `-`, removes leading and trailing separators, and limits the result to
63 characters. HWT then UTF-8 percent-encodes substituted values except the RFC
3986 unreserved set (`A-Z`, `a-z`, `0-9`, `-._~`). Use `{sanitized_branch}` in a
hostname. Literal braces are not supported.

HWT rejects malformed templates, invalid percent escapes, and templates without
a URL scheme.

## Service URLs

A URL entry with `service` returns the complete generated
`HWT_URL_<SERVICE>` value:

```yaml
urls:
  local:
    service: web
```

The service must be present in `ports.services`. See
[services and ports](../services-and-ports/) for local URL generation, custom
hostnames, and optional managed DNS.

## JSON output

`--json` prints the resolved name, URL, and optional label without opening it:

```sh
hwt url preview --json
```

```json
{
  "name": "preview",
  "url": "https://feature-name.preview.example.com",
  "label": "Branch preview"
}
```

`hwt url --json` resolves all available names into a sorted array. If the branch
has no pull request, the array omits URLs that require pull request metadata.
Resolving one such URL by name still reports the missing pull request.

## Flags

| Flag                    | Description                                           |
| ----------------------- | ----------------------------------------------------- |
| `--cwd PATH`            | Repository path. Defaults to the current directory.   |
| `-R, --repo REPOSITORY` | GitHub base repository in `[HOST/]OWNER/REPO` format. |
| `--json`                | Print machine-readable output.                        |
| `--open`                | Open the resolved HTTP(S) URL in the default browser. |

Without a branch argument, detached HEAD cannot identify a branch-dependent URL;
pass the branch explicitly.

## Migrate from `preview_url`

```yaml
# Before
preview_url: https://{sanitized_branch}.preview.example.com

# After
urls:
  preview: https://{sanitized_branch}.preview.example.com
```
