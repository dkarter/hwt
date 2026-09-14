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

- [`hwt create`](../cli/create-worktree/) creates and configures a Herdr worktree workspace.
- [`hwt copy`](../cli/copy-files/) copies configured files into a linked worktree once.
- [`hwt remove`](../cli/remove-worktree/) quickly removes a linked worktree and workspace.
- [`hwt list`](../cli/list-worktrees/) lists Herdr worktrees for a repository.
- [`hwt url NAME`](../worktree-urls/) resolves configured worktree, pull request, preview, and service URLs.
- [`hwt review`](../cli/review-pull-request/) fetches a pull request or branch into a dedicated review workspace.
- [`hwt env`](../worktree-environment/) generates, inspects, refreshes, or uses the current worktree environment.

## Experimental local DNS

The dnsmasq and Caddy integration is experimental. See
[services and ports](../services-and-ports/#optional-managed-dns) before setting
it up.

| Command                          | Purpose                                                                |
| -------------------------------- | ---------------------------------------------------------------------- |
| `dns setup [--json]`             | Generate HWT-owned dnsmasq and Caddy snippets and print include paths. |
| `dns status [--json]`            | Inspect generated paths and active worktree route registrations.       |
| `dns refresh [--cwd, --json]`    | Reconcile the current worktree route with its existing assigned ports. |
| `dns teardown [--force, --json]` | Remove only HWT-owned state; refuse active routes unless forced.       |

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

[`hwt skill`](../agent-skill/) prints the bundled agent instructions;
`hwt skill config` prints the separate configuration reference.

## Miscellaneous

- [Shell completions](../misc/shell-completions/) for Bash, Zsh, Fish, and PowerShell.
