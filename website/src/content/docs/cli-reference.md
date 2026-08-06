---
title: CLI reference
description: Commands and flags available in hwt.
---

## Global flags

| Flag               | Description                                      |
| ------------------ | ------------------------------------------------ |
| `--herdr-bin PATH` | Herdr executable to invoke. Defaults to `herdr`. |
| `--help`           | Show command help.                               |
| `--version`        | Print the hwt version.                           |

## `hwt create`

Create and configure a Herdr worktree workspace.

```sh
hwt create --branch BRANCH [flags]
```

| Flag                  | Description                                         |
| --------------------- | --------------------------------------------------- |
| `-b, --branch BRANCH` | Branch to create. Required.                         |
| `--base REF`          | Base ref. Defaults to the current branch.           |
| `--cwd PATH`          | Repository path. Defaults to the current directory. |
| `--path PATH`         | Override the configured worktree path.              |
| `--label LABEL`       | Herdr workspace label.                              |
| `--focus`             | Focus the new workspace.                            |
| `--json`              | Print machine-readable output.                      |

## `hwt list`

List Herdr worktrees for a repository.

```sh
hwt list [--cwd PATH]
```

The result uses Herdr's JSON worktree-list response.

## `hwt remove`

Quickly remove a linked worktree and Herdr workspace. `rm` is an alias.

```sh
hwt remove [flags]
```

| Flag                 | Description                                      |
| -------------------- | ------------------------------------------------ |
| `-w, --workspace ID` | Workspace ID. Defaults to the current workspace. |
| `-f, --force`        | Remove a dirty or locked worktree.               |
| `--json`             | Print machine-readable output.                   |

## `hwt config`

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
