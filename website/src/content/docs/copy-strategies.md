---
title: Copy strategies
description: Choose normal copies, APFS copy-on-write clones, or symlinks.
---

Each `files.copy` entry may be a path string or an object with per-entry behavior.

## Direct copy

```yaml
files:
  copy:
    - .env.local
    - fixtures
```

Direct copies are independent from the source checkout and work across supported filesystems.

## Copy on write

```yaml
files:
  copy_on_write: true
  copy:
    - deps
```

Copy on write is experimental and disabled by default. On macOS, hwt asks APFS to clone the complete hierarchy with `clonefile(2)`. Unchanged data shares disk blocks while later writes remain independent.

If cloning is unsupported, crosses filesystems, or cannot replace the destination, hwt falls back to a normal copy.

## Symlink

```yaml
files:
  copy:
    - path: node_modules
      symlink: true
```

A symlink is nearly instant and uses no additional storage, but writes through it modify the source checkout. Moving or removing the source leaves a broken link.

An entry cannot enable both `copy_on_write` and `symlink`.

## Scheduling

Copy operations run in parallel by default. An entry with `parallel: false` waits for earlier parallel copies, runs alone, and blocks later copies until it finishes.

```yaml
files:
  parallel: true
  copy:
    - .env.local
    - path: generated-cache
      parallel: false
    - fixtures
```
