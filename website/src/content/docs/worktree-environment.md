---
title: Worktree environment
description: Generate and use environment variables for each linked worktree.
---

HWT writes generated values and configured variables to `.env.worktree` in each
linked worktree. Use this file to give development commands the correct worktree
identity, local ports, and service URLs.

## Configuration

```yaml
environment:
  variables:
    APP_URL: http://localhost:${HWT_PORT_WEB}
```

A repository `environment.variables` map replaces the global map rather than
merging it. Values may reference generated variables such as `${HWT_PORT_WEB}`.
HWT does not read the parent process, `.env`, or `.env.local` while expanding
them.

## Generated values

Service names become uppercase variables with hyphens changed to underscores:

```dotenv
HWT_PORT_ASSETS="20001"
HWT_PORT_WEB="20000"
HWT_URL_ASSETS="http://example.assets.localhost:20001"
HWT_URL_WEB="http://example.web.localhost:20000"
HWT_WORKTREE_BRANCH="feature/example"
HWT_WORKTREE_HOSTNAME="example.localhost"
HWT_WORKTREE_PATH="/path/to/example"
APP_URL="http://localhost:20000"
```

`HWT_ENV_FILE` contains the absolute path to `.env.worktree`.
`HWT_WORKTREE_HOSTNAME` is the generic hostname and does not include a service or
port. `HWT_URL_<SERVICE>` is the complete URL. See
[services and ports](../services-and-ports/) for allocation and URL generation.

## Generate and use the environment

`hwt create` copies configured files, reserves ports, writes the environment,
then runs `post_create`. Hook commands receive the same variables in their
process environments. A failed create rolls back its reservation with the
worktree.

The Herdr `worktree.created` plugin event runs `hwt copy`; this also generates
the environment for worktrees created directly through Herdr. Repeated and
concurrent calls preserve the existing allocation.

`hwt env` regenerates the file from the existing allocation. Use `--json` for
structured output or run a command with the generated environment directly:

```sh
hwt env -- bin/dev
```

Inspect one value the same way:

```sh
hwt env -- printenv HWT_URL_WEB
```

Use `hwt env --refresh` after stopping development services if allocated ports
must be replaced. See [services and ports](../services-and-ports/#allocation)
for reservation lifecycle and conflict handling.

## Dotenv and secrets

`.env.worktree` has mode `0600`. HWT adds
`/.env.worktree` to the shared Git common directory's `info/exclude`, refuses
to overwrite a tracked file with that name, and rejects copying that exact
file through `files.copy`. The file contains generated identifiers, ports, and
literal `environment.variables` only.

HWT does not inspect, import, or merge `.env`, `.env.local`, copied files, or
the parent process environment. Existing dotenv files keep their current copy
behavior and precedence remains the application's responsibility. Load
`.env.worktree` after general dotenv files when generated values should win.
Do not put secrets in committed `environment.variables`; continue to use the
application's existing secret mechanism.

## Mise and Herdr

Projects using mise can opt in with tracked configuration:

```toml
[env]
_.file = ".env.worktree"
```

Mise activation then applies the generated values to commands and panes whose
shells activate mise. HWT deliberately does not generate or modify
`mise.local.toml`: executable directory configuration has a trust boundary,
could conflict with existing local policy, and would create another generated
file that users might commit. `hwt env -- COMMAND...` is the tool-independent
alternative.

Herdr cannot inject these values into the already-running root pane at creation
time. HWT generates the file immediately afterward. A mise-activated shell picks
it up on environment refresh, while newly launched commands can use mise or
`hwt env --`.
