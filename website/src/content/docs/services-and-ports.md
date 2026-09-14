---
title: Services and ports
description: Allocate stable local ports and URLs for services in each worktree.
---

HWT gives every configured service a stable, collision-free local port for each
linked worktree.

```yaml
ports:
  start: 20000
  end: 39999
  services: [web, assets]
```

A repository `ports.services` list may place `<global>` where globally configured
services should be inserted. The default inclusive range is `20000` to `39999`.

## Localhost URLs

Each service receives a direct URL that works without system configuration:

```dotenv
HWT_PORT_WEB="20000"
HWT_URL_WEB="http://feature-login.web.localhost:20000"
HWT_PORT_ASSETS="20001"
HWT_URL_ASSETS="http://feature-login.assets.localhost:20001"
```

RFC 6761 reserves `localhost` names for loopback resolution. These URLs require
no `sudo`, daemon, dnsmasq, Caddy, or hosts-file entry. HWT writes them to the
[worktree environment](../worktree-environment/). A named
[worktree URL](../worktree-urls/#service-urls) can expose the same complete value.

Customize direct URLs with `ports.url_template`. It supports normalized
`{worktree}` and `{service}` labels, the generic `{hostname}`, and `{port}`:

```yaml
ports:
  services: [web]
  url_template: http://{service}.{hostname}:{port}
```

For an existing wildcard domain where ports distinguish worktrees:

```yaml
ports:
  services: [web]
  url_template: https://app.acme.dev:{port}
```

HWT only generates the URL. The service must provide HTTPS for that hostname, or
a separately configured proxy must listen on the allocated port and terminate
TLS.

## Allocation

HWT stores allocations in
`${XDG_STATE_HOME:-~/.local/state}/hwt/ports.json`. Allocation and cleanup hold
an advisory process lock so concurrent HWT processes do not select the same port.
Each service receives the first free port in the configured range, after HWT
checks that the port is not already in use.

The registry contains only canonical worktree paths and integer ports. It does
not contain environment values or credentials. Exact ports depend on active
reservations and other listeners, not service order alone.

Reservations prevent conflicts among HWT worktrees but cannot hold an operating
system socket until a future server starts. If another process claims a recorded
port, stop the affected development services and run:

```sh
hwt env --refresh
```

Refresh replaces all ports for the current worktree. `hwt remove` releases its
allocation. Before a new allocation, HWT also removes entries whose worktree path
no longer exists.

## Optional managed DNS

:::caution[Experimental]
The dnsmasq and Caddy integration is experimental and may be replaced by
built-in HWT routing in a future release.
:::

Managed local DNS replaces direct localhost URLs with portless service URLs:

```yaml
ports:
  services: [web, assets]
local_dns:
  enabled: true
  domain: hwt.test
```

HWT derives a stable base hostname from the canonical repository and worktree
paths. A result resembles `app-feature-login-a1b2c3d4e5f6.hwt.test`. The path
hash prevents repositories or normalized names from silently sharing a hostname.
Each service is routed at `<service>.<base-hostname>` through Caddy to its
allocated port.

`hwt dns setup` creates these HWT-owned files under
`${XDG_STATE_HOME:-~/.local/state}/hwt/local-dns/`:

- `dnsmasq.conf` contains wildcard loopback mappings for configured local domains.
- `Caddyfile` contains exact service hosts and their `127.0.0.1:<port>` upstreams.
- `registry.json` contains canonical paths, hostnames, and service ports.

HWT does not install packages, edit `/etc`, invoke `sudo`, start listeners, or
manage host services. Include the reported snippets from separately installed
dnsmasq and Caddy services. Managed DNS is supported on macOS and Linux.

| Command                              | Purpose                                                           |
| ------------------------------------ | ----------------------------------------------------------------- |
| `hwt dns setup [--json]`             | Generate snippets and report their include paths.                 |
| `hwt dns status [--json]`            | Inspect generated paths and active registrations.                 |
| `hwt dns refresh [--cwd, --json]`    | Reconcile the current worktree route without replacing its ports. |
| `hwt dns teardown [--force, --json]` | Remove HWT-owned state; refuse active routes unless forced.       |

An optional `local_dns.reload` argv command can reload user-managed services
after generated files change. HWT runs it directly without a shell. Arguments
may contain `{caddyfile}`, `{dnsmasq}`, and `{state_dir}`. Failed reloads restore
the previous registry and generated files.

Delete worktrees through `hwt remove` so routes are cleaned up. A checkout
deleted by another tool keeps its route because HWT cannot confirm successful
cleanup; inspect it with `hwt dns status`.
