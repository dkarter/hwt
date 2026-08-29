---
title: Copy configured files
description: Prepare files in a linked worktree created directly through Herdr.
---

## Usage

```sh
hwt copy [--cwd PATH] [--json]
```

`hwt copy` loads configuration from the primary checkout and copies its
configured `files.copy` entries into a linked worktree. This allows an untracked
`.herdr-worktree.yaml` or `.yml` to include and copy itself. When the primary
checkout has no project config, HWT uses the shared Git-local config at
`<git-common-dir>/hwt/config.yaml` or `.yml`.

The command is intended for Herdr's `worktree.created` event and does not run
HWT's `post_create` commands.

The command automatically uses `workspace_cwd` from Herdr plugin context. When
there is no plugin context, it uses the current directory. Pass `--cwd` to
select a worktree explicitly.

## Copy once

HWT records completion in the linked worktree's private Git directory. Calls
for the same worktree are serialized, and every call after the first successful
copy is a fast no-op. A failed copy is not marked complete and can be retried.

This is a lifecycle command rather than a synchronization command. It does not
copy later source changes into an already prepared worktree.

## JSON output

```sh
hwt copy --json
```

The result includes the source and destination paths, copied entries, and an
`already_prepared` field indicating whether the operation was skipped.
