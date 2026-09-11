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
ticket_command: [lnr, quick, --json]
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-

urls:
  preview: https://{sanitized_branch}.preview.example.com
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
```

## Resolution rules

Project or Git-local scalar values override global values. Repository lists replace global lists unless they contain `<global>` at the position where global entries should be inserted. `environment.variables` replaces the global map as a unit. Named URLs, static metadata, and metadata commands merge by name with repository entries winning.

The `<global>` marker is valid in repository `files.copy`, `ports.services`, and `post_create` lists. It cannot appear in the global configuration.

## Fields

### `agent`

Command Herdr starts in the root pane after creation.

### `ticket_command`

Argument array used by `hwt create DESCRIPTION`. The default is `[lnr, quick, --json]`. Repository configuration replaces the global array as a whole. HWT appends the full description as one final argument and runs the executable directly, without a shell. The command must write one JSON object to stdout with a non-empty string `branchName`, for example `{"branchName":"team/rms-90-task","metadata":{"identifier":"RMS-90"}}`. Only the optional dedicated string-valued `metadata` object becomes URL metadata; other top-level fields are ignored. Diagnostics belong on stderr; a non-zero exit includes its status and stderr in hwt's error.

### `worktree_dir`

Directory where worktrees are created. hwt resolves relative paths from the repository root before invoking Herdr.

### `worktree_naming`

Controls the checkout name. Accepted values are `full` and `basename`; the default is `full`.

### `worktree_prefix`

Text prepended to the generated checkout name.

### `urls` and `metadata`

`urls` maps arbitrary names to absolute URL templates. Global and repository
maps merge by name, with repository entries winning. Built-in placeholders are:

| Placeholder          | Value                                                                 |
| -------------------- | --------------------------------------------------------------------- |
| `{repository}`       | Primary checkout directory name.                                      |
| `{branch}`           | Current branch, or the explicit branch argument.                      |
| `{sanitized_branch}` | Branch normalized for preview hostnames and identifiers.              |
| `{worktree}`         | Current checkout directory name; unavailable with an explicit branch. |
| `{hostname}`         | Stable HWT local hostname; unavailable with an explicit branch.       |
| `{pr_number}`        | GitHub pull request number resolved lazily with authenticated `gh`.   |

Sanitization lowercases ASCII letters, replaces each run of characters outside
`a-z` and `0-9` with one `-`, removes leading and trailing separators, and limits
the result to 63 characters. HWT
then UTF-8 percent-encodes every substituted value except the RFC 3986
unreserved set (`A-Z`, `a-z`, `0-9`, `-._~`). Use `{sanitized_branch}` in a
hostname; all placeholders are safe as a path segment or query value. Literal
braces are not supported. Configuration validation rejects malformed templates,
invalid percent escapes, and templates without a URL scheme.

`metadata.values` provides static strings. A command under
`metadata.commands.NAME` is an argv array run directly without a shell only when
the template requests `{NAME.key}`. Built-in placeholders in individual command
arguments (`repository`, `branch`, `sanitized_branch`, `worktree`, and
`hostname`) are
substituted as one argument; custom metadata, `pr_number`, and ambient
environment variables are not expanded. The command must return one JSON object
whose values are strings.

Ticket commands may return a dedicated string-valued `metadata` object alongside
`branchName`. HWT exposes it as `{ticket.key}` for that worktree. It ignores
arbitrary top-level ticket fields. Ticket metadata is stored in a mode-`0600`
file under private Git worktree metadata and removed with the worktree. Command
output and resolved URLs, including credentials, are never persisted.

Do not put credentials in tracked `metadata.values`. Use a lazy metadata command
for database passwords, tokens, and other secrets.

Precedence is static values, then `ticket.*`, then command namespaces, then
reserved built-ins. Unknown and missing placeholders fail resolution. Explicit
branches cannot use `{worktree}`, `{hostname}`, or worktree-local `ticket.*`
values.

To migrate from the earlier `preview_url` form:

```yaml
# Before
preview_url: https://{sanitized_branch}.preview.example.com

# After
urls:
  preview: https://{sanitized_branch}.preview.example.com
```

### `files`

Controls files and directories transferred from the source checkout. Missing sources are ignored, and every path must stay inside the repository. See [copy strategies](/docs/copy-strategies/).

### `post_create`

Commands run in order after all copy operations finish. Empty commands are rejected.

### `ports`

Defines service names and the inclusive local allocation range. Service `web`
becomes `HWT_PORT_WEB`; hyphens become underscores. The defaults are `20000`
through `39999`. See [worktree ports and environment](/docs/worktree-environment/).

### `environment`

`environment.variables` contains non-secret values for `.env.worktree` and
`post_create`. Values may reference generated variables such as
`${HWT_PORT_WEB}`. HWT does not read values from the parent process, `.env`, or
`.env.local` while expanding them.

### `local_dns`

`local_dns.enabled` registers every `ports.services` entry in HWT-owned
dnsmasq and Caddy snippets. `domain` defaults to `hwt.test`. HWT validates and
lowercases DNS labels; it rejects `localhost`, path-like values, empty labels,
and labels longer than 63 bytes.

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
