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

## Performance benchmarks

Representative warm-cache measurements on macOS with same-volume APFS source and destination paths:

| Workload |    Size | Filesystem entries | Direct copy | Copy on write |  Symlink |
| -------- | ------: | -----------------: | ----------: | ------------: | -------: |
| A        | 703 MiB |             24,342 |      5.77 s |        0.45 s | ~0.005 s |
| B        | 737 MiB |             23,020 |      5.43 s |        0.69 s | ~0.005 s |
| C        | 1.7 GiB |            265,463 |     83.91 s |       13.77 s | ~0.005 s |

Each destination was absent before measurement. Direct copy uses hwt's recursive file copier, copy on write uses an APFS directory clone, and symlink creates one link without materializing the tree. Entry counts include files, directories, and symlinks. Results vary with filesystem, cache state, storage, and tree shape; these numbers illustrate the tradeoffs rather than guarantee performance.

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
