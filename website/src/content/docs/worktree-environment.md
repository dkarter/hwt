---
title: Worktree ports and environment
description: Stable local ports and generated environment variables for each linked worktree.
---

HWT gives each linked worktree stable local ports and writes its generated
values to `.env.worktree`.

## Selected design

HWT uses a persistent machine-local registry at
`${XDG_STATE_HOME:-~/.local/state}/hwt/ports.json`. Allocation and cleanup hold
an advisory process lock, so concurrent HWT processes cannot choose the same
port. Each configured service receives the first free port in the configured
inclusive range. HWT checks that a newly assigned TCP port is not already in
use before recording it.

This approach was selected over two simpler alternatives:

| Approach                                       | Benefit                                                                    | Why it was not selected                                                                                                          |
| ---------------------------------------------- | -------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Deterministic derivation from a branch or path | No stored state; reproducible calculation                                  | Hash collisions still need coordination, and the calculated port may be occupied by an unrelated process.                        |
| Dynamic operating-system allocation            | Finds a free port at allocation time                                       | The socket must be released before the development server starts, so the value is unstable and another process can win the race. |
| Persistent local registry                      | Stable values, atomic multi-worktree coordination, multiple named services | Requires stale-entry cleanup and remains advisory: non-HWT processes can claim a reserved port later.                            |

The registry contains only canonical worktree paths and integer ports. It never
contains environment values or credentials.

## Configuration

```yaml
ports:
  start: 20000
  end: 39999
  services:
    - web
    - assets

environment:
  variables:
    APP_URL: http://localhost:${HWT_PORT_WEB}
```

A repository `ports.services` list may place `<global>` where globally
configured services should be inserted. A repository
`environment.variables` map replaces the global map rather than merging it.

Service names become uppercase variables with hyphens changed to underscores:

```dotenv
HWT_PORT_ASSETS="20001"
HWT_PORT_WEB="20000"
HWT_WORKTREE_BRANCH="feature/example"
HWT_WORKTREE_PATH="/path/to/example"
APP_URL="http://localhost:20000"
```

`HWT_ENV_FILE` also contains the absolute path to `.env.worktree`. The exact
ports depend on active reservations and other listeners, not service order
alone.

## Lifecycle and conflicts

`hwt create` copies configured files, reserves ports, writes the environment,
then runs `post_create`. Hook commands receive the same variables in their
process environments. A failed create rolls back its reservation with the
worktree.

The Herdr `worktree.created` plugin event runs `hwt copy`; this also generates
the environment for worktrees created directly through Herdr. Repeated and
concurrent calls preserve the existing allocation.

`hwt env` regenerates the file from the existing allocation. Use
`hwt env --refresh` after stopping the affected development services if an
unrelated process has claimed a reserved port. Refresh discards all of that
worktree's old ports and allocates again. Use `hwt env --json` for structured
output or run a command directly:

```sh
hwt env -- bin/dev
```

`hwt remove` releases the allocation. Before every new allocation HWT also
removes entries whose worktree path no longer exists, covering worktrees
deleted outside HWT. Existing paths are retained because HWT cannot safely
infer that an idle worktree is abandoned.

The registry prevents conflicts among HWT worktrees and avoids ports already
bound at allocation time. It cannot reserve the operating-system socket for
the lifetime of a future server. A non-HWT process can still claim a recorded
port later; explicit refresh is the recovery path.

## Dotenv and secrets

`.env.worktree` is generated with mode `0600`. HWT adds
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

Herdr currently accepts environment variables when creating workspaces and
panes, but its `worktree create` command does not expose an environment option.
HWT therefore cannot inject values into the already-running root pane at
creation time. The file is generated immediately afterward; a mise-activated
shell picks it up on environment refresh, and newly launched development
commands can use mise or `hwt env --`. Direct root-pane injection should wait
for a Herdr worktree-create environment API rather than sending shell-specific
`export` text into an interactive pane.

## Caddy and dnsmasq

RMS-87 can read `HWT_PORT_<SERVICE>` from `.env.worktree` or the `variables`
object returned by `hwt env --json`, then generate a Caddy upstream such as
`127.0.0.1:${HWT_PORT_WEB}`. dnsmasq only maps the worktree hostname to a local
address; it does not need the upstream port. Proxy configuration must refresh
after `hwt env --refresh` and be removed during worktree cleanup.
