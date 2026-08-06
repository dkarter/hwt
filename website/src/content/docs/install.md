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

## Select another Herdr binary

Every command accepts `--herdr-bin` when Herdr is not on your `PATH` or you want to test another build:

```sh
hwt --herdr-bin /path/to/herdr list
```
