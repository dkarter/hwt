---
title: Worktree ports, URLs, and environment
description: Stable local ports, development hostnames, and generated environment variables for each linked worktree.
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
HWT_URL_ASSETS="http://assets.example.localhost:20001"
HWT_URL_WEB="http://web.example.localhost:20000"
HWT_WORKTREE_BRANCH="feature/example"
HWT_WORKTREE_HOSTNAME="example.localhost"
HWT_WORKTREE_PATH="/path/to/example"
APP_URL="http://localhost:20000"
```

`HWT_ENV_FILE` also contains the absolute path to `.env.worktree`. The exact
ports depend on active reservations and other listeners, not service order
alone.

## Zero-setup localhost URLs

By default, each configured service also receives a direct HTTP URL with its
allocated port. HWT normalizes the worktree directory name into a DNS label and
uses it under `.localhost`:

```text
HWT_WORKTREE_HOSTNAME=feature-login.localhost
HWT_URL_WEB=http://web.feature-login.localhost:20000
HWT_URL_ASSETS=http://assets.feature-login.localhost:20001
```

RFC 6761 reserves `localhost` names for loopback resolution. These URLs need no
sudo, daemon, dnsmasq, Caddy, hosts-file entry, or operating-system setup. They
remain stable while the worktree keeps its allocated ports. Use the URL variable
directly in application configuration or inspect it with:

```sh
hwt env -- printenv HWT_URL_WEB
```

`HWT_WORKTREE_HOSTNAME` is the shared base hostname and does not include a
service or port. `HWT_URL_<SERVICE>` is the complete browser URL for that
service. Hyphens in service names become underscores in variable names.

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

## Optional Caddy and dnsmasq

Managed local DNS is opt-in and replaces the default direct localhost URLs:

```yaml
ports:
  services: [web, assets]
local_dns:
  enabled: true
  domain: hwt.test
```

HWT derives a stable base hostname from the primary repository directory and
worktree directory, plus the first 12 hexadecimal characters of SHA-256 over
their canonical paths. Human-readable components are lowercased, runs outside
ASCII `a-z0-9` become `-`, empty components use `repo` or `worktree`, and the
readable prefix is shortened as needed so the complete first DNS label remains
at most 63 bytes. A result resembles
`app-feature-login-a1b2c3d4e5f6.hwt.test`. The path hash prevents two
repositories or normalized names from silently sharing a hostname.

Each service is routed at `<service>.<base-hostname>`, for example
`http://web.app-feature-login-a1b2c3d4e5f6.hwt.test`. Service labels use the
same lowercase ASCII sanitization; configurations where two names sanitize to
the same label are rejected. `.env.worktree` exposes the base hostname as
`HWT_WORKTREE_HOSTNAME` and each service URL as `HWT_URL_<SERVICE>`. The
`{hostname}` named-URL placeholder exposes the same base hostname, so a project
can configure `urls.local: http://web.{hostname}` and use `hwt url local`.
Managed URLs omit ports because Caddy proxies each hostname to its allocated
service port. This behavior is unchanged and `{hostname}` remains available only
when `local_dns.enabled` is true.

`hwt dns setup` creates and reports these HWT-owned files under
`${XDG_STATE_HOME:-~/.local/state}/hwt/local-dns/`:

- `dnsmasq.conf` contains only wildcard loopback mappings for configured local
  domains.
- `Caddyfile` contains only exact service hosts and their current
  `127.0.0.1:<port>` upstreams.
- `registry.json` contains canonical paths, hostnames, and service ports.

HWT does not install packages, edit `/etc`, modify an existing Caddyfile, invoke
`sudo`, start listeners, or manage host services. The user includes these two
snippets from existing dnsmasq and Caddy installations. This is smaller and
safer than making normal worktree creation privileged, and it cannot overwrite
unrelated configuration.

Use `hwt dns status --json` to inspect paths and registrations, `hwt dns
refresh` to reconcile the current worktree without replacing its ports, and
`hwt dns teardown` after all registrations are removed. Teardown removes the
registry and generated snippets but retains the empty lock directory for safe
concurrent use. It refuses active registrations unless `--force` is explicit.

An optional `local_dns.reload` argv command can reload user-managed services
after a snippet changes. HWT runs it directly without a shell. Arguments may
contain `{caddyfile}`, `{dnsmasq}`, and `{state_dir}`. Failed reloads restore the
previous registry and generated files; setup, registration, refresh, and
cleanup are idempotent and serialized across repositories.

`hwt create`, direct-Herdr `hwt copy`, and `hwt env` register current routes.
`hwt env --refresh` updates routes to the replacement ports. `hwt remove`
removes routes only after the checkout and Git metadata cleanup succeeds; if
the route reload fails, HWT restores the previous route and reports that the
worktree itself was already removed.

Delete worktrees through `hwt remove`. A checkout deleted by another tool keeps
its route because HWT cannot confirm that cleanup succeeded; inspect it with
`hwt dns status` and use `hwt dns teardown --force` only when deliberately
discarding all registrations.

Only macOS and Linux are supported. Both require separately installed dnsmasq
and Caddy services configured to include the reported snippets. HWT reports an
unsupported-platform error elsewhere. The default `.test` suffix is reserved
for private testing; changing it requires a valid multi-label DNS name and must
not use `localhost`.
