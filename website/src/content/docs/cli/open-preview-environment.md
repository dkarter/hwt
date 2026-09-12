---
title: Open a preview environment
description: Resolve and open a configured preview URL for a worktree or branch.
---

Set `urls.preview` in the repository, Git-local, or global configuration, then run:

```sh
hwt url preview
```

HWT uses the current branch and worktree, expands the template, validates the
result, and prints it. Pass a branch to resolve a preview
that is not checked out locally:

```sh
hwt url preview feature/name
```

An explicit branch has no associated worktree, so its template cannot use
`{worktree}` or `ticket.*`. See [configuration](/docs/configuration/#urls-and-metadata) for the
placeholder, sanitization, and escaping contract.

## Machine-readable output

`--json` prints the resolved URL and does not open a browser:

```sh
hwt url preview --json
```

```json
{
  "name": "preview",
  "url": "https://feature-name.preview.example.com"
}
```

## Flags

| Flag                    | Description                                           |
| ----------------------- | ----------------------------------------------------- |
| `--cwd PATH`            | Repository path. Defaults to the current directory.   |
| `--json`                | Print the resolved URL without opening a browser.     |
| `--open`                | Open the resolved HTTP(S) URL in the default browser. |
| `-R, --repo REPOSITORY` | GitHub repository used to resolve `{pr_number}`.      |

Without a branch argument, detached HEAD cannot identify a preview; pass the
branch explicitly. Add `--open` to open the URL. On macOS hwt uses `open`; on
Linux it uses `xdg-open`.

HWT does not poll or wait for a deployment. Preview providers differ in status
APIs, authentication, and readiness semantics, so this command only resolves and
opens the configured URL.

Use `hwt url NAME [branch]` for any named URL. It prints plain output by default,
`--json` prints `{"name":"...","url":"..."}`, and `--open` explicitly opens
only HTTP(S) URLs. Database and other non-browser schemes are never opened by
default and are rejected with `--open`.
