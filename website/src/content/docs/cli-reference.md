---
title: CLI overview
description: Commands available in hwt and links to detailed reference pages.
---

## Global flags

| Flag               | Description                                      |
| ------------------ | ------------------------------------------------ |
| `--herdr-bin PATH` | Herdr executable to invoke. Defaults to `herdr`. |
| `--help`           | Show command help.                               |
| `--version`        | Print the hwt version.                           |

## Worktrees

- [`hwt create`](/docs/cli/create-worktree/) creates and configures a Herdr worktree workspace.
- [`hwt remove`](/docs/cli/remove-worktree/) quickly removes a linked worktree and workspace.
- [`hwt list`](/docs/cli/list-worktrees/) lists Herdr worktrees for a repository.

## Configuration

| Command                  | Description                                                   |
| ------------------------ | ------------------------------------------------------------- |
| `config path [--global]` | Print a repository or global config path.                     |
| `config show`            | Print resolved configuration and sources as JSON.             |
| `config validate [path]` | Validate a file or the resolved repository config.            |
| `config init [--global]` | Create a starter config without overwriting an existing file. |

## `hwt schema`

Print the embedded JSON Schema. The output always matches the schema used by that hwt binary.

```sh
hwt schema
```

## `hwt skill`

Print the canonical hwt skill for AI agent discovery:

```sh
hwt skill
```

The core skill keeps project configuration details out of routine worktree
operations. Print that on-demand reference separately:

```sh
hwt skill config
```

## Miscellaneous

- [Shell completions](/docs/misc/shell-completions/) for Bash, Zsh, Fish, and PowerShell.
