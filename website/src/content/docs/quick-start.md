---
title: Quick start
description: Create and remove your first configured Herdr worktree.
---

Run these commands inside a Git repository.

## Create a workspace

```sh
hwt create --branch feat/my-change
```

hwt uses the current branch as the base, asks Herdr to create a linked worktree and workspace, applies your configuration, and leaves focus where it is.

Focus the new workspace immediately with `--focus`:

```sh
hwt create --branch feat/my-change --focus
```

## Add repository setup

Generate `.herdr-worktree.yaml`:

```sh
hwt config init
```

Then describe what each new checkout needs:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json
agent: opencode --port
files:
  copy:
    - .env.local
post_create:
  - mise install
```

Validate the resolved global and repository configuration:

```sh
hwt config validate
```

## Inspect worktrees

```sh
hwt list
```

## Remove the workspace

From inside the Herdr workspace:

```sh
hwt remove
```

From elsewhere, provide its workspace ID:

```sh
hwt remove --workspace w_8Jx2
```

hwt refuses to remove a dirty or locked worktree. Review the checkout before using `--force`.
