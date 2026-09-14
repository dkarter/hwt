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

## Development builds

The standard `mise use -g github:dkarter/hwt` command excludes GitHub prereleases by default, so development builds do not change stable installations. Each development release provides an exact command like this in its release notes and workflow summary:

```sh
mise use -g github:dkarter/hwt@0.6.2-dev.20260911.42.1.gabcdef1
```

Use that exact version to test a build. Verify it with `hwt --version`, and include the full version and commit when [reporting an issue](https://github.com/dkarter/hwt/issues).

Advanced users can include prereleases in fuzzy version listings and `latest` resolution by adding this to their mise TOML configuration:

```toml
[tools]
"github:dkarter/hwt" = { version = "latest", prerelease = true }
```

## Use hwt from Herdr

The Herdr plugin adds interactive actions for creating and removing
configured worktrees without leaving Herdr.

[Install and configure the Herdr plugin →](../herdr-plugin/)
