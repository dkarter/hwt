---
title: Agent skill
description: Install the hwt agent skill or print its bundled instructions from the CLI.
---

The hwt agent skill teaches supported coding agents to create, inspect, and
remove Herdr-managed worktrees safely. It also includes an on-demand reference
for editing `.herdr-worktree.yaml`.

:::note
The skill does not install the `hwt` binary. [Install hwt](/docs/install/) before
asking an agent to use it.
:::

## Install with Aube

[`aubx`](https://aube.jdx.dev/package-manager/scripts.html#one-off-binaries)
runs a matching local `skills` binary when available. Otherwise, it installs and
runs the CLI in a throwaway project:

```sh
aubx skills add dkarter/hwt
```

`aubx` ships with [Aube](https://aube.jdx.dev/), so install Aube first if the
command is not available. The skills CLI prompts you to select target agents
and an installation method. It installs into the current project by default;
add `--global` for a user-wide installation.

## Install with npm

Run the same skills CLI through `npx` if you use npm:

```sh
npx skills add dkarter/hwt
```

Both commands discover the `hwt` skill directly from the repository. See the
[skills CLI documentation](https://skills.sh/docs/cli) for agent selection,
global installation, and non-interactive options.

## Print the bundled skill

The hwt binary embeds the canonical skill so agents and integrations can read
instructions that match the installed version:

```sh
hwt skill
```

The core skill keeps project configuration details out of routine worktree
operations. Print that on-demand reference separately:

```sh
hwt skill config
```
