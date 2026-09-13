# Project Configuration

Use this reference only when building or changing a repository's hwt config.

## Workflow

1. Inspect the repository's tool manifests, ignored local files, dependency
   directories, and setup commands.
2. Generate `.herdr-worktree.yaml` with `hwt config init` if it does not exist.
   Use `hwt config init --git-common` for a machine-local repository config
   shared by all linked worktrees. Never overwrite an existing config.
3. Add only project-specific overrides. Global defaults load first.
4. Run `hwt config validate`, then inspect the merged result with
   `hwt config show`.

Use the schema shipped with the installed binary when exact fields matter:

```bash
hwt schema
```

## Template

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json
agent: opencode --port
# Optional example for Linear users:
ticket_commands:
  default:
    command: [lnr, issue, search, --json, '{input}']
    output:
      branch: branchName
      metadata: { identifier: issueId, title: title, url: url }
  create:
    command: [lnr, quick, '{input}', --json]
    output:
      branch: branchName
      metadata: { identifier: issueId, title: title, url: url }
review_command: [tuicr]
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-

urls:
  preview:
    template: https://{sanitized_branch}.preview.example.com
    label: Branch preview
  local: http://web.{hostname}
  ticket: https://linear.example/issue/{ticket.identifier}

metadata:
  values:
    region: us-east-1
  commands:
    deployment: [bin/deployment-metadata, --branch, '{branch}', --json]

ports:
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
    - path: node_modules
      copy_on_write: true

post_create:
  - <global>
  - mise install
```

Remove fields the project does not need. When no project config exists, hwt uses
`<git-common-dir>/hwt/config.yaml` or `.yml` as the repository config. Repository
scalar values override global values. Repository `files.copy` and `post_create`
replace global lists unless `<global>` appears where the global entries should
be inserted.

## Choosing Settings

- `agent`: Set the command an orchestrator should start in the root pane.
- `ticket_commands`: Optionally configure named ticket command objects. Plain
  `--ticket` selects `default`; `--ticket=NAME` selects another. `{input}` places
  the optional positional value in argv without a shell. An exact placeholder
  argument is omitted when no input is supplied. Stdout is reserved for final
  JSON, so interactive UI must use stderr. Output selectors map JSON
  string fields to the branch and persisted ticket metadata. Global and
  repository commands merge by name, with repository entries winning.
- `review_command`: Set the argv launched by `hwt review`; the default is
  `[tuicr]`. Repository configuration replaces the global argv. HWT adds no PR,
  branch, title, or URL arguments and shell-quotes each configured argument
  before asking Herdr to run it in the review checkout.
- `worktree_dir`: Resolve relative values from the repository root.
- `worktree_naming`: Use `full` to preserve branch hierarchy in the checkout
  name, or `basename` to use only the final branch component.
- `worktree_prefix`: Add a stable project prefix when checkout names could
  collide.
- `urls`: Map names to absolute template strings or objects with `template` and
  an optional display `label` for `hwt url NAME`. HWT provides a GitHub `pr` URL
  by default; configure `urls.pr` to replace it for another forge. Built-ins are
  `{repository}`, `{branch}`, `{sanitized_branch}`,
  `{worktree}`, `{hostname}`, `{pr_host}`, `{pr_owner}`, `{pr_repository}`, and
  `{pr_number}`. Explicit branches cannot use worktree-local values.
- `metadata.values`: Add static strings. Repository values override global ones.
- `metadata.commands`: Map a namespace to direct argv. A command runs lazily for
  `{namespace.key}`, receives repository/branch/worktree substitutions as safe
  individual arguments, and must return one JSON object with string values. No shell or
  ambient environment expansion occurs. Ticket command `metadata` is exposed as
  `ticket.*`; command output and resolved URLs are never persisted.
- `files.copy`: Copy ignored, machine-local inputs needed immediately, such as
  `.env.local`. Missing sources are ignored. Do not list tracked files.
- `copy_on_write`: Prefer for large dependency trees on filesystems that support
  cloning. It safely falls back to a normal copy.
- `symlink: true`: Use only when writes may intentionally affect the source
  checkout. Never combine it with `copy_on_write`.
- `parallel: false`: Use on an entry that must run alone or after earlier copies.
- `post_create`: Run deterministic setup commands from the new worktree root,
  such as dependency installation or code generation. Commands run in order
  after all file operations finish.
- `ports`: List local services that need stable, distinct ports. HWT exposes
  them as `HWT_PORT_<SERVICE>` and `HWT_URL_<SERVICE>` in `.env.worktree` and
  `post_create`. With local DNS disabled, URLs use RFC 6761 localhost subdomains
  and allocated ports without system setup. Set `url_template` to customize the
  direct URL with `{worktree}`, `{service}`, `{hostname}`, and `{port}`.
- `local_dns`: Opt in to HWT-owned dnsmasq and Caddy snippets. Run
  `hwt dns setup` once, include the reported files from user-managed services,
  and use `hwt dns status` to inspect registrations. Leave it disabled for
  zero-setup `.localhost` URLs. `reload` is an optional argv command; HWT never
  edits system configuration or invokes `sudo`.
- `environment.variables`: Add non-secret values. Generated HWT variables may
  be referenced with `${NAME}` when available before configured expansion, such
  as `HWT_PORT_<SERVICE>`. Consume hostname and URL variables directly from the
  generated environment. Never put credentials in project config.

All copy paths must stay within the repository. Prefer setup commands over
copying generated state when regeneration is reliable and reasonably fast.
