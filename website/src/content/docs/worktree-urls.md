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
Set an entry to `false` to disable a default or inherited URL. A more specific
URL definition re-enables it.

## Cache URLs

The built-in `pr` and `repo` URLs are cached for five minutes. A missing pull
request is cached for 15 seconds, so a newly created pull request appears
quickly without making every URL listing wait on GitHub.

Enable caching for any template URL with `cache: true`, or set custom durations:

```yaml
urls:
  preview:
    template: https://{deployment.host}
    cache:
      ttl: 10m
      negative_ttl: 15s
```

The cache stores the final URL under the user cache directory. Because metadata
commands can return credentials, caching is opt-in for custom URLs. Do not cache
a URL that may contain secrets.

Use `--refresh` to bypass and update the cache. Set `cache: false` when overriding
a built-in URL to disable its cache.

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

## GitHub defaults

HWT provides `repo` and `pr` by default for GitHub repositories:

```sh
hwt url repo
hwt url pr
hwt url pr feature/name
```

The `repo` URL remains available when the current branch has no pull request.
HWT resolves repository and pull request details through the authenticated `gh` CLI. With
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

Disable either default globally or per repository when it is not useful:

```yaml
urls:
  pr: false
  repo: false
```

To fetch code into a dedicated Herdr workspace and launch a review tool instead,
use [`hwt review`](../cli/review-pull-request/).

## Placeholders

| Placeholder          | Value                                                                    |
| -------------------- | ------------------------------------------------------------------------ |
| `{repository}`       | Primary checkout directory name.                                         |
| `{branch}`           | Current branch, or the explicit branch argument.                         |
| `{sanitized_branch}` | Branch normalized for hostnames and identifiers.                         |
| `{worktree}`         | Current checkout directory name; unavailable with an explicit branch.    |
| `{hostname}`         | Managed local DNS hostname; requires `local_dns.enabled` and a worktree. |
| `{repo_host}`        | GitHub repository host resolved lazily with authenticated `gh`.          |
| `{repo_owner}`       | GitHub repository owner resolved lazily with authenticated `gh`.         |
| `{repo_repository}`  | GitHub repository name resolved lazily with authenticated `gh`.          |
| `{pr_host}`          | GitHub pull request host resolved lazily with authenticated `gh`.        |
| `{pr_owner}`         | GitHub pull request owner resolved lazily with authenticated `gh`.       |
| `{pr_repository}`    | GitHub pull request repository resolved lazily with authenticated `gh`.  |
| `{pr_number}`        | GitHub pull request number resolved lazily with authenticated `gh`.      |

[Worktree metadata](../worktree-metadata/) adds static, ticket, and lazy command
placeholders such as `{ticket.identifier}` and `{database.host}`.

The `{hostname}` placeholder is not the default `.localhost` value in
`HWT_WORKTREE_HOSTNAME`. To expose a default localhost URL through `hwt url`, use
a [service URL](#service-urls) instead.

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
has no pull request, the array omits URLs that require pull request metadata and
still includes `repo` when GitHub can identify the repository.
Resolving one such URL by name still reports the missing pull request.

## Flags

| Flag                    | Description                                           |
| ----------------------- | ----------------------------------------------------- |
| `--cwd PATH`            | Repository path. Defaults to the current directory.   |
| `-R, --repo REPOSITORY` | GitHub base repository in `[HOST/]OWNER/REPO` format. |
| `--json`                | Print machine-readable output.                        |
| `--open`                | Open the resolved HTTP(S) URL in the default browser. |
| `--refresh`             | Bypass and update configured URL caches.              |

Without a branch argument, detached HEAD cannot identify a branch-dependent URL;
pass the branch explicitly.
