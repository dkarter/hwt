<p align="center">
  <img src="website/public/favicon.svg" width="64" height="64" alt="hwt worktree mark">
</p>

# hwt

`hwt` turns a Git branch into a configured [Herdr](https://herdr.dev) workspace, ready for you or a coding agent to start work.

**[Website](https://hwt.doriankarter.com/) · [Documentation](https://hwt.doriankarter.com/docs/)**

## Features

- Create consistently named and placed worktrees with one command.
- Copy, APFS-clone, or symlink the files and dependencies each checkout needs.
- Run setup commands before handing the workspace to a person or agent.
- Allocate collision-free ports and stable local URLs per worktree.
- Create review workspaces for pull requests and remote branches.
- Automate safely with structured JSON output and bundled agent instructions.
- Create and remove configured worktrees without leaving Herdr.

## Install

```bash
mise use -g github:dkarter/hwt
```

## Quick start

Create a worktree and Herdr workspace from the current branch:

```bash
hwt create feature/my-change
```

Add a `.herdr-worktree.yaml` file to define the checkout name and copy files that are not tracked by Git:

```yaml
worktree_naming: basename

files:
  copy:
    - .env.local
```

For example, `feature/my-change` creates a checkout named `my-change` with `.env.local` copied from the primary checkout.

<details>
<summary>Complete configuration example</summary>

```yaml
# Optional: tell Herdr which agent command to start in the new workspace.
agent: opencode --port

# Optional: create branches from ticket tools and save selected ticket metadata.
ticket_commands:
  default:
    command: [lnr, issue, search, --json, '{input}']
    output: &ticket-output
      branch: branchName
      metadata:
        identifier: issueId
        title: title
        url: url
  create:
    command: [lnr, quick, '{input}', --json]
    output: *ticket-output

# Optional: choose the command launched by `hwt review` (defaults to tuicr).
# An exact {pr_url} argument expands to the canonical pull request URL for PR reviews.
review_command: [tuicr, pr, '{pr_url}']

# Optional: control where worktrees live and how their directory names are generated.
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-

# Optional: define links available through `hwt url` (GitHub repository and pull request links work by default).
urls:
  preview:
    template: https://{sanitized_branch}.preview.example.com
    label: Branch preview
    cache: true
  local: http://web.{hostname}
  ticket: https://linear.example/issue/{ticket.identifier}
  database: postgres://{database.user}:{database.password}@{database.host}/app

# Optional: provide static or command-backed values used in named URLs.
metadata:
  values:
    region: us-east-1
  commands:
    database: [bin/database-metadata, --branch, '{branch}', --json]

# Optional: reserve per-worktree ports and generate direct local service URLs.
ports:
  start: 20000
  end: 39999
  services: [web, assets]
  url_template: http://{worktree}.{service}.localhost:{port}

# Optional: generate dnsmasq and Caddy routes instead of using direct localhost URLs.
local_dns:
  enabled: true
  domain: hwt.test

# Optional: write non-secret values to .env.worktree and expose them during setup.
environment:
  variables:
    APP_URL: http://localhost:${HWT_PORT_WEB}

# Optional: copy, clone, or symlink untracked files and dependencies into each worktree.
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

# Optional: run setup commands after all configured file operations finish.
post_create:
  - <global>
  - mise install
```

The schema is available at [`schema/herdr-worktree.schema.json`](schema/herdr-worktree.schema.json). See the [configuration guide](https://hwt.doriankarter.com/docs/configuration/) for defaults, placeholders, and resolution rules.

</details>

Remove it from inside its workspace:

```bash
hwt remove
```

See the [quick start](https://hwt.doriankarter.com/docs/quick-start/) for configuration, validation, and other common workflows.

## Herdr plugin

Install the official plugin to create and safely remove configured worktrees from Herdr:

```bash
hwt plugin install
```

The plugin adds **New configured worktree** and **Remove current worktree** actions. Refresh it after updating hwt with `hwt plugin update`.

## Documentation

The [documentation](https://hwt.doriankarter.com/docs/) covers configuration, copy strategies, worktree environments, the CLI, and agent integration.

Run `hwt skill` to print the bundled usage instructions for coding agents.

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for building, testing, and releasing hwt.

## License

MIT
