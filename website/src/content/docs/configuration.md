---
title: Configuration
description: Configure worktree placement, file transfer, agents, and setup commands.
---

hwt reads global defaults first, then repository policy.

| Scope            | Path                                               |
| ---------------- | -------------------------------------------------- |
| Global           | `${XDG_CONFIG_HOME:-~/.config}/hwt/config.yaml`    |
| Git-local        | `<git-common-dir>/hwt/config.yaml` or `config.yml` |
| Project checkout | `.herdr-worktree.yaml` or `.herdr-worktree.yml`    |

Create these files with `hwt config init --global`, `hwt config init --git-common`, or `hwt config init`.

The project checkout config takes precedence. HWT uses the Git-local config only
when neither project checkout file exists. Git stores it outside the checkout,
so it remains machine-local while being available to every linked worktree.

## Complete example

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json
agent: opencode --port
# Optional example for Linear users:
ticket_commands:
  default:
    command: [lnr, issue, search, --json, '{input}']
    output:
      branch: branchName
      metadata:
        identifier: issueId
        title: title
        url: url
  create:
    command: [lnr, quick, '{input}', --json]
    output:
      branch: branchName
      metadata:
        identifier: issueId
        title: title
        url: url
review_command: [tuicr]
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-

urls:
  pr: https://gitlab.example/group/project/-/merge_requests?source_branch={branch}
  preview:
    template: https://{sanitized_branch}.preview.example.com
    label: Branch preview
  local: http://web.{hostname}
  ticket: https://linear.example/issue/{ticket.identifier}
  database: postgres://{database.user}:{database.password}@{database.host}/app

metadata:
  values:
    region: us-east-1
  commands:
    database: [bin/database-metadata, --branch, '{branch}', --json]

ports:
  start: 20000
  end: 39999
  services: [web, assets]
  url_template: http://{worktree}.{service}.localhost:{port}

local_dns:
  enabled: true
  domain: hwt.test

environment:
  variables:
    APP_URL: http://localhost:${HWT_PORT_WEB}

files:
  parallel: true
  copy_on_write: false
  copy:
    - <global>
    - .env.local
    - path: deps
      copy_on_write: true
    - path: node_modules
      parallel: false
      symlink: true

post_create:
  - <global>
  - mise install

pre_remove:
  - pitchfork stop --local
post_remove:
  - pitchfork clean --prune
```

## Resolution rules

Project or Git-local scalar values override global values. Named `ticket_commands` merge by name with repository entries winning. Repository `review_command` replaces the global array as a whole. Other lists replace global lists unless they contain `<global>` at the position where global entries should be inserted. `environment.variables` replaces the global map as a unit. Named URLs, static metadata, and metadata commands merge by name with repository entries winning. A repository URL replaces the complete global entry, including its label.

The `<global>` marker is valid in repository `files.copy`, `ports.services`, `post_create`, `pre_remove`, and `post_remove` lists. It cannot appear in the global configuration.

## Fields

### `agent`

Command Herdr starts in the root pane after creation.

### `ticket_commands`

Optional named commands used by `hwt create --ticket[=NAME]`. The plain
`--ticket` flag selects the `default` entry. Each entry contains a direct argv
and optional output selectors:

```yaml
ticket_commands:
  default:
    command: [lnr, issue, search, --json, '{input}']
    output:
      branch: branchName
      metadata:
        identifier: issueId
        title: title
        url: url
  create:
    command: [lnr, quick, '{input}', --json]
    output:
      branch: branchName
      metadata:
        identifier: issueId
        title: title
        url: url
```

HWT replaces `{input}` in place without invoking a shell. If no input is
supplied, an argument that is exactly `{input}` is omitted. An embedded form
such as `--query={input}` requires input. Commands without `{input}` receive no
additional argument. Command stdout is reserved for final JSON, so interactive
picker UI must use stderr. For pipelines or transformations beyond field selection,
configure `sh -c` explicitly and pass `{input}` after a `$0` placeholder:

```yaml
ticket_commands:
  transformed:
    command:
      - sh
      - -c
      - |
          another-ticket-cli create --title "$1" --json |
            jq '{branchName: .git.branch, metadata: {identifier: .key}}'
      - hwt-ticket
      - '{input}'
```

The command must print one JSON object. `output.branch` defaults to
`branchName`; metadata maps stored names to source selectors. Selectors use dot
notation for nested objects and must resolve to strings. If no metadata mapping
is configured, HWT accepts a dedicated string-valued `metadata` object. Named
commands from global and repository configuration merge with repository entries
winning.

### `review_command`

Argument array launched by `hwt review` from the review checkout. The default is
`[tuicr]`, and repository configuration replaces the global array as a whole.
An argument that is exactly `{pr_url}` expands to the canonical pull request URL.
For example, `[tuicr, pr, '{pr_url}']` opens tuicr in pull request mode without a
commit picker. The placeholder is rejected for branch reviews, which have no pull
request URL. Other arguments remain literal. HWT shell-quotes each argument before
submitting the command to the Herdr pane, so no metadata is interpreted by the shell.

HWT verifies that the executable is available before launch. A successful
`launched` result means Herdr accepted the command; the tool then owns the pane.
If it exits immediately or later returns non-zero, the worktree and workspace
remain available and a later `hwt review` relaunches it. An open matching
workspace returns `already_open` without starting a duplicate only while its
recorded review pane remains busy.

### `worktree_dir`

Directory where worktrees are created. hwt resolves relative paths from the repository root before invoking Herdr.

### `worktree_naming`

Controls the checkout name. Accepted values are `full` and `basename`; the default is `full`.

### `worktree_prefix`

Text prepended to the generated checkout name.

### `urls`

`urls` maps arbitrary names to URL templates or generated service URLs. Values
can be template strings or objects with `template` or `service`. Objects may
include a display `label`:

```yaml
urls:
  local:
    service: web
    label: Local app
  preview:
    template: https://{sanitized_branch}.preview.example.com
    label: Branch preview
```

HWT includes a `pr` URL for GitHub by default. Global and repository maps merge
by name, with repository entries winning. See [worktree URLs](../worktree-urls/)
for placeholders, service URLs, pull request resolution, and CLI usage.

### `metadata`

`metadata.values` provides static strings. A command under
`metadata.commands.NAME` is an argv array run directly without a shell only when
the template requests `{NAME.key}`. The command must return one JSON object whose
values are strings. See [worktree metadata](../worktree-metadata/) for static,
ticket, and lazy metadata behavior, precedence, storage, and security.

### `files`

Controls files and directories transferred from the source checkout. Missing sources are ignored, and every path must stay inside the repository. See [copy strategies](../copy-strategies/).

### `post_create`

Commands run in order after all copy operations finish. Empty commands are rejected.

### `pre_remove`

Commands run in order from the linked worktree root before HWT renames the checkout or closes its Herdr workspace. Commands receive the generated worktree environment. A failure aborts removal and leaves the checkout and workspace intact.

### `post_remove`

Commands run in order from the primary checkout after HWT removes the checkout metadata and releases local DNS routes and ports. Commands retain the removed worktree's generated environment. A failure is reported, but the removal is already complete.

### `ports`

Defines service names and the inclusive local allocation range. Service `web`
becomes `HWT_PORT_WEB` and `HWT_URL_WEB`; hyphens become underscores. Without
managed local DNS, the URL uses an RFC 6761 localhost subdomain and the allocated
port with no system setup. The defaults are `20000` through `39999`. See
[services and ports](../services-and-ports/) for allocation, URL templates, and
managed DNS.

`ports.url_template` controls direct URLs when `local_dns.enabled` is false.

### `environment`

`environment.variables` contains non-secret values for `.env.worktree` and
`post_create`. Values may reference generated variables such as
`${HWT_PORT_WEB}` that are available before configured values are expanded.
Hostname and URL variables should be consumed directly from the generated
environment. HWT does not read values from the parent process, `.env`, or
`.env.local` while expanding them. See
[worktree environment](../worktree-environment/) for generated values, dotenv
handling, and command usage.

### `local_dns`

`local_dns.enabled` registers every `ports.services` entry in HWT-owned dnsmasq
and Caddy snippets. This integration is experimental. `domain` defaults to
`hwt.test`. Leave it disabled to use direct `.localhost` URLs with allocated
ports and no system setup. See
[services and ports](../services-and-ports/#optional-managed-dns) for setup and
lifecycle details.

`local_dns.reload` is an optional argv command run directly, without a shell,
after generated files change. Arguments may contain `{caddyfile}`, `{dnsmasq}`,
or `{state_dir}`. Repository values replace the global argv as a whole. Keep
privileged operations in an explicit user-owned wrapper if your service setup
requires them; HWT itself never invokes `sudo` or edits system files.

## Inspect and validate

```sh
hwt config path
hwt config path --git-common
hwt config path --global
hwt config show
hwt config validate
hwt config validate ./example.yaml
```

`config show` prints the fully resolved configuration and the source paths as JSON.
