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
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-

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

Project or Git-local scalar values override global values. Repository lists replace global lists unless they contain `<global>` at the position where global entries should be inserted.

The `<global>` marker is valid only in repository `files.copy` and `post_create` lists. It cannot appear in the global configuration.

## Fields

### `agent`

Command Herdr starts in the root pane after creation.

### `worktree_dir`

Directory where worktrees are created. hwt resolves relative paths from the repository root before invoking Herdr.

### `worktree_naming`

Controls the checkout name. Accepted values are `full` and `basename`; the default is `full`.

### `worktree_prefix`

Text prepended to the generated checkout name.

### `files`

Controls files and directories transferred from the source checkout. Missing sources are ignored, and every path must stay inside the repository. See [copy strategies](/docs/copy-strategies/).

### `post_create`

Commands run in order after all copy operations finish. Empty commands are rejected.

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
