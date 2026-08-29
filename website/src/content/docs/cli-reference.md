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

### Select another Herdr binary

Every command accepts `--herdr-bin` when Herdr is not on your `PATH` or you want
to test another build:

```sh
hwt --herdr-bin /path/to/herdr list
```

## Worktrees

- [`hwt create`](/docs/cli/create-worktree/) creates and configures a Herdr worktree workspace.
- [`hwt copy`](/docs/cli/copy-files/) copies configured files into a linked worktree once.
- [`hwt remove`](/docs/cli/remove-worktree/) quickly removes a linked worktree and workspace.
- [`hwt list`](/docs/cli/list-worktrees/) lists Herdr worktrees for a repository.

## Configuration

| Command                                | Description                                                   |
| -------------------------------------- | ------------------------------------------------------------- |
| `config path [--global, --git-common]` | Print a project, Git-local, or global config path.            |
| `config show`                          | Print resolved configuration and sources as JSON.             |
| `config validate [path]`               | Validate a file or the resolved repository config.            |
| `config init [--global, --git-common]` | Create a starter config without overwriting an existing file. |

## Herdr plugin

| Command            | Description                                    |
| ------------------ | ---------------------------------------------- |
| `plugin install`   | Install the official HWT plugin through Herdr. |
| `plugin update`    | Refresh the installed HWT plugin.              |
| `plugin uninstall` | Remove the HWT plugin from Herdr.              |

## `hwt schema`

Print the embedded JSON Schema. The output always matches the schema used by that hwt binary.

```sh
hwt schema
```

## Agent integration

[`hwt skill`](/docs/agent-skill/) prints the bundled agent instructions;
`hwt skill config` prints the separate configuration reference.

## Miscellaneous

- [Shell completions](/docs/misc/shell-completions/) for Bash, Zsh, Fish, and PowerShell.
