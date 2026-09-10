---
title: Open a preview environment
description: Resolve and open a configured preview URL for a worktree or branch.
---

Set `preview_url` in the repository, Git-local, or global configuration, then run:

```sh
hwt preview
```

HWT uses the current branch and worktree, expands the template, validates the
result, and opens it in the default browser. Pass a branch to resolve a preview
that is not checked out locally:

```sh
hwt preview feature/name
```

An explicit branch has no associated worktree, so its template cannot use
`{worktree}`. See [configuration](/docs/configuration/#preview_url) for the
placeholder, sanitization, and escaping contract.

## Machine-readable output

`--json` prints the resolved URL and does not open a browser:

```sh
hwt preview --json
```

```json
{
  "url": "https://feature-name.preview.example.com"
}
```

## Flags

| Flag         | Description                                         |
| ------------ | --------------------------------------------------- |
| `--cwd PATH` | Repository path. Defaults to the current directory. |
| `--json`     | Print the resolved URL without opening a browser.   |

Without a branch argument, detached HEAD cannot identify a preview; pass the
branch explicitly. On macOS hwt opens URLs with `open`; on Linux it uses
`xdg-open`.

HWT does not poll or wait for a deployment. Preview providers differ in status
APIs, authentication, and readiness semantics, so this command only resolves and
opens the configured URL.
