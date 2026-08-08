---
title: Install
description: Install hwt and verify its Herdr dependency.
---

## Requirements

hwt requires:

- Git
- [Herdr](https://herdr.dev/docs/install/)
- macOS or Linux

## Install with mise

```sh
mise use -g github:dkarter/hwt
```

Confirm that both commands are available:

```sh
herdr --version
hwt --version
```

## Build from source

```sh
git clone https://github.com/dkarter/hwt.git
cd hwt
mise install
mise run install
```

The install task writes the binary to `~/.local/bin/hwt`. Ensure that directory is on your `PATH`.

## Use hwt from Herdr

The experimental Herdr plugin adds interactive actions for creating and removing
configured worktrees without leaving Herdr.

[Install and configure the Herdr plugin →](/docs/herdr-plugin/)
