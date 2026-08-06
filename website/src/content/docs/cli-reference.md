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

## Shell completions

hwt can generate completion scripts for Bash, Zsh, Fish, and PowerShell.

### Bash

Install and initialize [`bash-completion`](https://github.com/scop/bash-completion) first. Cobra's generated Bash script depends on it.

Load completions in the current shell:

```bash
source <(hwt completion bash)
```

Install them for future sessions:

```bash
mkdir -p ~/.local/share/bash-completion/completions
hwt completion bash > ~/.local/share/bash-completion/completions/hwt
```

### Zsh

Add a completion directory to `fpath` in `~/.zshrc`:

```zsh
fpath=(~/.zfunc $fpath)
autoload -Uz compinit
compinit
```

Then generate the completion file:

```zsh
mkdir -p ~/.zfunc
hwt completion zsh > ~/.zfunc/_hwt
```

Start a new shell or run `exec zsh` after installing it.

### Fish

```fish
mkdir -p ~/.config/fish/completions
hwt completion fish > ~/.config/fish/completions/hwt.fish
```

Fish discovers the file automatically in new and existing sessions.

### PowerShell

Load completions in the current session:

```powershell
hwt completion powershell | Out-String | Invoke-Expression
```

Add the same command to `$PROFILE` to load completions in future sessions.
