---
title: Configuration
description: Configure worktree placement, file transfer, agents, and setup commands.
---

hwt reads global defaults first, then repository policy.

| Scope            | Path                                               |
| ---------------- | -------------------------------------------------- |
| Global           | `${XDG_CONFIG_HOME:-~/.config}/hwt/config.yaml`    |
| Git-local        | `<git-common-dir>/hwt/config.yaml` or `config.yml` |
| Project checkout | `.herdr-worktree.yaml` or `.herdr-worktree.yml`    |

Create these files with `hwt config init --global`, `hwt config init --git-common`, or `hwt config init`.

The project checkout config takes precedence. HWT uses the Git-local config only
when neither project checkout file exists. Git stores it outside the checkout,
so it remains machine-local while being available to every linked worktree.

## Complete example

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json
agent: opencode --port
ticket_command: [lnr, quick, --json]
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-
preview_url: https://{sanitized_branch}.preview.example.com

ports:
  start: 20000
  end: 39999
  services: [web, assets]

environment:
  variables:
    APP_URL: http://localhost:${HWT_PORT_WEB}

files:
  parallel: true
  copy_on_write: false
  copy:
    - <global>
    - .env.local
    - path: deps
      copy_on_write: true
    - path: node_modules
      parallel: false
      symlink: true

post_create:
  - <global>
  - mise install
```

## Resolution rules

Project or Git-local scalar values override global values. Repository lists replace global lists unless they contain `<global>` at the position where global entries should be inserted. `environment.variables` replaces the global map as a unit.

The `<global>` marker is valid in repository `files.copy`, `ports.services`, and `post_create` lists. It cannot appear in the global configuration.

## Fields

### `agent`

Command Herdr starts in the root pane after creation.

### `ticket_command`

Argument array used by `hwt create DESCRIPTION`. The default is `[lnr, quick, --json]`. Repository configuration replaces the global array as a whole. HWT appends the full description as one final argument and runs the executable directly, without a shell. The command must write one JSON object to stdout with a non-empty string `branchName`, for example `{"branchName":"team/rms-90-task"}`. Diagnostics belong on stderr; a non-zero exit includes its status and stderr in hwt's error.

### `worktree_dir`

Directory where worktrees are created. hwt resolves relative paths from the repository root before invoking Herdr.

### `worktree_naming`

Controls the checkout name. Accepted values are `full` and `basename`; the default is `full`.

### `worktree_prefix`

Text prepended to the generated checkout name.

### `preview_url`

Absolute HTTP(S) URL template used by `hwt preview [branch]`. It is a scalar, so
repository configuration replaces the global value. Supported placeholders are:

| Placeholder          | Value                                                                 |
| -------------------- | --------------------------------------------------------------------- |
| `{repository}`       | Primary checkout directory name.                                      |
| `{branch}`           | Current branch, or the explicit branch argument.                      |
| `{sanitized_branch}` | Branch normalized for preview hostnames and identifiers.              |
| `{worktree}`         | Current checkout directory name; unavailable with an explicit branch. |

Sanitization lowercases ASCII letters, replaces each run of characters outside
`a-z` and `0-9` with one `-`, removes leading and trailing separators, and limits
the result to 63 characters. HWT
then UTF-8 percent-encodes every substituted value except the RFC 3986
unreserved set (`A-Z`, `a-z`, `0-9`, `-._~`). Use `{sanitized_branch}` in a
hostname; all placeholders are safe as a path segment or query value. Literal
braces are not supported. Configuration validation rejects malformed templates,
unknown placeholders, invalid percent escapes, and templates that are not
absolute HTTP(S) URLs.

### `files`

Controls files and directories transferred from the source checkout. Missing sources are ignored, and every path must stay inside the repository. See [copy strategies](/docs/copy-strategies/).

### `post_create`

Commands run in order after all copy operations finish. Empty commands are rejected.

### `ports`

Defines service names and the inclusive local allocation range. Service `web`
becomes `HWT_PORT_WEB`; hyphens become underscores. The defaults are `20000`
through `39999`. See [worktree ports and environment](/docs/worktree-environment/).

### `environment`

`environment.variables` contains non-secret values for `.env.worktree` and
`post_create`. Values may reference generated variables such as
`${HWT_PORT_WEB}`. HWT does not read values from the parent process, `.env`, or
`.env.local` while expanding them.

## Inspect and validate

```sh
hwt config path
hwt config path --git-common
hwt config path --global
hwt config show
hwt config validate
hwt config validate ./example.yaml
```

`config show` prints the fully resolved configuration and the source paths as JSON.
