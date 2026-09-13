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
```

## Resolution rules

Project or Git-local scalar values override global values. Named `ticket_commands` merge by name with repository entries winning. Repository `review_command` replaces the global array as a whole. Other lists replace global lists unless they contain `<global>` at the position where global entries should be inserted. `environment.variables` replaces the global map as a unit. Named URLs, static metadata, and metadata commands merge by name with repository entries winning. A repository URL replaces the complete global entry, including its label.

The `<global>` marker is valid in repository `files.copy`, `ports.services`, and `post_create` lists. It cannot appear in the global configuration.

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
HWT does not append the pull request URL, number, branch, title, or other remote
metadata. It shell-quotes each configured argument before submitting the command
to the Herdr pane, so arguments remain literal and no metadata is interpreted by
the shell.

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

### `urls` and `metadata`

`urls` maps arbitrary names to absolute URL templates. A value can be a template
string or an object with a required `template` and optional display `label`:

```yaml
urls:
  local: http://web.{hostname}
  preview:
    template: https://{sanitized_branch}.preview.example.com
    label: Branch preview
```

HWT includes a `pr` URL for GitHub by default. Global and repository maps merge
by name, with repository entries winning, so an `urls.pr` entry can target
GitLab, Forgejo, or another forge. An override replaces the complete entry
rather than inheriting its label. Shell completion reads the merged names for
the current repository.
Built-in placeholders are:

| Placeholder          | Value                                                                   |
| -------------------- | ----------------------------------------------------------------------- |
| `{repository}`       | Primary checkout directory name.                                        |
| `{branch}`           | Current branch, or the explicit branch argument.                        |
| `{sanitized_branch}` | Branch normalized for preview hostnames and identifiers.                |
| `{worktree}`         | Current checkout directory name; unavailable with an explicit branch.   |
| `{hostname}`         | Stable HWT local hostname; unavailable with an explicit branch.         |
| `{pr_host}`          | GitHub pull request host resolved lazily with authenticated `gh`.       |
| `{pr_owner}`         | GitHub pull request owner resolved lazily with authenticated `gh`.      |
| `{pr_repository}`    | GitHub pull request repository resolved lazily with authenticated `gh`. |
| `{pr_number}`        | GitHub pull request number resolved lazily with authenticated `gh`.     |

Sanitization lowercases ASCII letters, replaces each run of characters outside
`a-z` and `0-9` with one `-`, removes leading and trailing separators, and limits
the result to 63 characters. HWT
then UTF-8 percent-encodes every substituted value except the RFC 3986
unreserved set (`A-Z`, `a-z`, `0-9`, `-._~`). Use `{sanitized_branch}` in a
hostname; all placeholders are safe as a path segment or query value. Literal
braces are not supported. Configuration validation rejects malformed templates,
invalid percent escapes, and templates without a URL scheme.

`hwt url NAME [branch]` prints one resolved URL. Add `--open` for HTTP(S) URLs.
JSON output includes `label` when the URL entry configures one and omits the
field otherwise. `hwt url --json` computes every configured URL and prints a
sorted array of name, URL, and optional label objects. If the current branch has
no pull request, it omits URLs that require pull request metadata. Other
resolution errors still fail the command. Resolving a PR-dependent URL by name
still reports the missing pull request.

`metadata.values` provides static strings. A command under
`metadata.commands.NAME` is an argv array run directly without a shell only when
the template requests `{NAME.key}`. Built-in placeholders in individual command
arguments (`repository`, `branch`, `sanitized_branch`, `worktree`, and
`hostname`) are
substituted as one argument; custom metadata, `pr_number`, and ambient
environment variables are not expanded. The command must return one JSON object
whose values are strings.

A ticket command without output metadata mappings may return a dedicated string-valued `metadata` object
alongside `branchName`. HWT exposes it as `{ticket.key}` for that worktree. It
ignores arbitrary top-level fields. Ticket metadata is stored in a mode-`0600`
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

Controls files and directories transferred from the source checkout. Missing sources are ignored, and every path must stay inside the repository. See [copy strategies](../copy-strategies/).

### `post_create`

Commands run in order after all copy operations finish. Empty commands are rejected.

### `ports`

Defines service names and the inclusive local allocation range. Service `web`
becomes `HWT_PORT_WEB` and `HWT_URL_WEB`; hyphens become underscores. Without
managed local DNS, the URL uses an RFC 6761 localhost subdomain and the allocated
port with no system setup. The defaults are `20000` through `39999`. See
[worktree ports and environment](../worktree-environment/).

`ports.url_template` controls direct URLs when `local_dns.enabled` is false. It
supports `{worktree}`, `{service}`, `{hostname}`, and `{port}`. The first two
values are normalized DNS labels, while `{hostname}` is the generic
`<worktree>.localhost` hostname. For a company wildcard loopback domain where
ports distinguish worktrees, use:

```yaml
ports:
  services: [web]
  url_template: https://app.acme.dev:{port}
```

This publishes the URL only. The process on the allocated port must terminate
TLS itself, or a separately configured proxy must listen on that port.

### `environment`

`environment.variables` contains non-secret values for `.env.worktree` and
`post_create`. Values may reference generated variables such as
`${HWT_PORT_WEB}` that are available before configured values are expanded.
Hostname and URL variables should be consumed directly from the generated
environment. HWT does not read values from the parent process, `.env`, or
`.env.local` while expanding them.

### `local_dns`

`local_dns.enabled` registers every `ports.services` entry in HWT-owned
dnsmasq and Caddy snippets. `domain` defaults to `hwt.test`. HWT validates and
lowercases DNS labels; it rejects `localhost`, path-like values, empty labels,
and labels longer than 63 bytes. Leave it disabled to use direct `.localhost`
URLs with allocated ports and no dnsmasq or Caddy setup.

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
