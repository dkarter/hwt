<p align="center">
  <img src="website/public/favicon.svg" width="64" height="64" alt="hwt worktree mark">
</p>

# hwt

`hwt` is a CLI companion to [herdr](https://herdr.dev) that removes the friction from Git worktrees. Turn a branch into a configured Herdr workspace with files copied, dependencies linked, and setup finished. The same result for you and every agent.

**[Website](https://hwt.doriankarter.com/) · [Documentation](https://hwt.doriankarter.com/docs/)**

## Install

```bash
mise use -g github:dkarter/hwt
```

Build from source with `mise run build` or install to `~/.local/bin/hwt` with `mise run install`.

## Herdr plugin

Install the official plugin to create and safely remove HWT worktrees from Herdr:

```bash
hwt plugin install
```

Refresh it after updating HWT with `hwt plugin update`, or remove it with
`hwt plugin uninstall`.

The plugin adds **New configured worktree** and **Remove current worktree** actions. It also prepares configured files when a worktree is created directly through Herdr. It uses portable commands built into `hwt` and does not require a separate fuzzy finder or command palette.

Optional keybindings:

```toml
[[keys.command]]
key = "prefix+shift+g"
type = "plugin_action"
command = "hwt.worktrees.new"
description = "new configured worktree"

[[keys.command]]
key = "prefix+shift+d"
type = "plugin_action"
command = "hwt.worktrees.remove"
description = "remove configured worktree"
```

When developing locally, use `herdr plugin link ./plugins/herdr` instead.

## Commands

```bash
hwt create feature/name
hwt create --ticket=create 'describe the work to do'
hwt create --branch feature/name --base main --json
hwt copy
hwt env
hwt env --json
hwt env --refresh
hwt env -- bin/dev
hwt env -- printenv HWT_URL_WEB
hwt dns setup --json
hwt dns status --json
hwt list
hwt url ticket
hwt url database --json
hwt url preview --open
hwt url preview feature/name --json
hwt url pr --open
hwt url pr feature/name --json
hwt url --json
hwt review https://github.com/owner/repository/pull/123 --json
hwt review origin/feature/name --focus
hwt remove --workspace w1A --json
hwt config show
hwt config validate
hwt config init
hwt config init --git-common
hwt config init --global
hwt herdr create
hwt herdr remove
hwt plugin install
hwt plugin update
hwt plugin uninstall
hwt schema
hwt skill
hwt skill config
hwt completion zsh
```

`hwt create VALUE` uses `VALUE` as the literal branch name. `hwt create --ticket [INPUT]` runs `ticket_commands.default`, while `--ticket=NAME` selects another named command. HWT substitutes the optional input for `{input}` in configured argv, reads the branch and metadata through each command's output selectors, and creates that branch through the normal Herdr flow. An exact `{input}` argument is omitted when no input is supplied, allowing interactive selectors such as `lnr issue search --json`. Use `--branch BRANCH` for the explicit flag form. Creation defaults to the current Git branch as its base and does not change focus. Its JSON result includes the workspace ID, root pane ID, checkout path, base branch, copied files, and configured agent command.

`hwt copy` copies configured files from the primary checkout into the current linked worktree once. It reads Herdr plugin event context automatically, and concurrent or repeated calls are safe no-ops.

`hwt env` creates or refreshes the ignored `.env.worktree` file and reports its path. Pass `--json` to inspect values, `--refresh` to replace allocated ports, or `-- COMMAND...` to run a development command with the variables in its process environment. Without managed local DNS, every configured service gets a zero-setup RFC 6761 URL such as `http://feature-login.web.localhost:20000`; these `.localhost` names resolve to loopback without sudo, a daemon, or OS configuration. Customize the direct URL with `ports.url_template`, for example `https://app.acme.dev:{port}` for a wildcard loopback domain and a service that terminates TLS on its allocated port.

`hwt dns setup` generates HWT-owned dnsmasq and Caddy snippets under the XDG state directory for user-managed services. When `local_dns.enabled` is set, environment creation registers stable per-service URLs, refresh updates upstream ports, and successful worktree removal unregisters routes. HWT never edits system configuration, invokes `sudo`, or manages listeners.

`hwt remove` refuses dirty or locked worktrees unless `--force` is provided. It quickly renames the checkout out of the way, closes the Herdr workspace, removes Git's worktree metadata, and deletes the checkout in the background.

`hwt review SELECTOR` creates a dedicated review workspace for a full HTTPS GitHub pull request URL or a branch reference. Pull requests are fetched through the base repository's `refs/pull/NUMBER/head`, so same-repository and fork heads work without changing the primary checkout. Local branches are used as already fetched; `REMOTE/BRANCH`, `--remote`, or a single configured remote selects remote branches. HWT creates a deterministic `hwt/review/...` branch, verifies its exact commit before reuse, opens the linked worktree in Herdr without focusing it by default, and launches `review_command`. An exact workspace whose recorded review pane is still busy is returned without launching a duplicate tool; an idle session is relaunched. Use `--focus` to switch workspaces and `--json` for identity, commit, path, workspace/pane IDs, reuse state, and launch status.

`hwt url NAME [branch]` resolves a configured named URL and prints it without opening anything. `--json` returns its name and URL plus an optional display label; without a name it returns available URLs in sorted order and omits PR-dependent URLs when the branch has no pull request. `--open` explicitly opens only HTTP(S) URLs. HWT provides a GitHub `pr` URL by default, and `urls.pr` can replace it for GitLab, Forgejo, or another forge. URL names from the current repository are available to shell completion. Explicit branches need not exist locally, but cannot use `{worktree}` or worktree-local `ticket.*` metadata.

`hwt skill` prints the canonical usage skill for AI agents. Its concise core points agents to `hwt skill config`, which prints the project-configuration reference only when needed.

## Configuration

Global defaults live at `${XDG_CONFIG_HOME:-~/.config}/hwt/config.yaml`. A repository may override them with `.herdr-worktree.yaml` or `.herdr-worktree.yml` at its root. When neither project file exists, hwt falls back to `<git-common-dir>/hwt/config.yaml` or `.yml`, which is machine-local and shared by every linked worktree.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json
agent: opencode --port
# Optional examples for Linear users:
ticket_commands:
  default:
    command: [lnr, issue, search, --json, '{input}']
    output: &linear-output
      branch: branchName
      metadata:
        identifier: issueId
        title: title
        url: url
  create:
    command: [lnr, quick, '{input}', --json]
    output: *linear-output
review_command: [tuicr]
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-
urls:
  # Override the default GitHub pull request URL for another forge:
  # pr: https://gitlab.example/group/project/-/merge_requests?source_branch={branch}
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

Project values override global values. Named `ticket_commands` merge by name, with repository entries winning; `review_command` is replaced as a whole. Ticket command arguments substitute `{input}` directly without a shell. Command stdout is reserved for final JSON; an interactive picker must render its UI on stderr. An exact `{input}` argument is omitted when no input is supplied, while an embedded placeholder such as `--query={input}` requires input. HWT never appends pull request or branch metadata to `review_command`. Every review argument is shell-quoted before the command is submitted to the Herdr pane. Other project lists replace global lists unless they contain `<global>` at the position where global entries should be inserted. This includes `ports.services`; `environment.variables` replaces the global map as a unit. Named URLs, static metadata, and metadata commands merge by name. Missing copy sources are ignored. Copy paths must remain within the repository.

Named `urls` support `{repository}`, `{branch}`, `{sanitized_branch}`, `{worktree}`, `{hostname}`, `{pr_host}`, `{pr_owner}`, `{pr_repository}`, `{pr_number}`, static metadata, `ticket.*` metadata, and command-backed metadata. Pull request placeholders are resolved through authenticated `gh`; templates based only on `{branch}` work without it. A command named `database` returns a JSON object of string values exposed as `{database.key}`. Commands run directly as configured argv, only when their namespace is requested; built-ins in arguments are substituted without a shell. HWT does not expand ambient environment variables.

Global and repository URL/metadata maps merge by name, with repository entries winning. During resolution static values load first, persisted `ticket.*` values override matching static values, command namespace values override both, and reserved built-ins win last. Ticket metadata comes from the selected ticket command's configured output mappings, or its dedicated `metadata` object when no mappings are configured. It is stored mode `0600` in private per-worktree Git state and removed with that worktree. Command results and resolved URLs are never persisted.

Do not put credentials in tracked `metadata.values`. Use a lazy metadata command for database passwords, tokens, and other secrets.

Repository is the primary checkout directory name; worktree is the current checkout directory name. Sanitization lowercases ASCII letters, replaces runs outside `a-z0-9` with `-`, trims separators, and limits the result to 63 characters. Every substitution is UTF-8 percent-encoded except RFC 3986 unreserved characters (`A-Z`, `a-z`, `0-9`, `-._~`). Unknown, malformed, or missing placeholders are rejected before output or browser opening. To migrate, replace the `preview_url` key with a `preview` entry under `urls`.

HWT reserves stable, collision-free ports in `${XDG_STATE_HOME:-~/.local/state}/hwt/ports.json` under a process lock. It writes `HWT_PORT_WEB`, `HWT_URL_WEB`, `HWT_WORKTREE_HOSTNAME`, the worktree path and branch, and configured non-secret variables to a mode-`0600` `.env.worktree`. The generated file is added to Git's repository-local exclude file and is available to `post_create`; see the [environment design](website/src/content/docs/worktree-environment.md) for lifecycle, security, mise, Herdr, and reverse-proxy details.

Copy entries may be path strings or objects. Strings inherit the `files.parallel` and `files.copy_on_write` defaults; object entries can override either setting. Copies run in parallel by default and all finish before post-create commands run. An entry with `parallel: false` waits for prior parallel copies, runs alone, and blocks later copies until it finishes.

`copy_on_write` is experimental and disabled by default. On macOS, hwt asks APFS to clone the entire file or directory hierarchy with `clonefile(2)`, so unchanged file data shares disk blocks with the source. If cloning is unsupported, crosses filesystems, or cannot replace an existing destination, hwt falls back to a normal copy. Writes remain independent after a successful clone and allocate new blocks as needed.

An entry with `symlink: true` replaces its destination with an absolute symlink to the source checkout instead of copying data. It cannot also enable `copy_on_write`. Writes through the symlink modify the source checkout, and moving or removing the source leaves a broken link.

### Copy Strategy Benchmarks

Representative warm-cache measurements on macOS with same-volume APFS source and destination paths:

| Workload |    Size | Filesystem entries | Direct copy | Copy on write |  Symlink |
| -------- | ------: | -----------------: | ----------: | ------------: | -------: |
| A        | 703 MiB |             24,342 |      5.77 s |        0.45 s | ~0.005 s |
| B        | 737 MiB |             23,020 |      5.43 s |        0.69 s | ~0.005 s |
| C        | 1.7 GiB |            265,463 |     83.91 s |       13.77 s | ~0.005 s |

Each destination was absent before measurement. Direct copy uses hwt's recursive file copier, copy on write uses an APFS directory clone, and symlink creates one link without materializing the tree. Entry counts include files, directories, and symlinks. Results vary with filesystem, cache state, storage, and tree shape; these numbers illustrate the tradeoffs rather than guarantee performance.

The schema is published at `schema/herdr-worktree.schema.json` and embedded in each binary. `hwt schema` prints the exact embedded schema, while `hwt config validate` performs strict YAML and semantic validation.

## Development

```bash
mise install
mise run check
mise run snapshot
```

Product behavior is specified under `openspec/specs/` and linked to compiled-binary E2E tests by
scenario IDs. See [the testing guide](docs/testing.md) for adding scenarios, running one linked test,
and diagnosing hermetic or live Herdr failures.

Releases are managed by Release Please and GoReleaser. CI tests on macOS and Linux and builds the CLI on every pull request. Every push to `main` publishes an immutable development prerelease, and maintainers can run the **Publish development build** workflow for another Git ref. Its version uses the next patch after the highest published stable SemVer release, followed by `-dev.YYYYMMDD.RUN.ATTEMPT.gSHA`; for example, stable `v0.6.1` produces `v0.6.2-dev.20260911.42.1.gabcdef1`. This keeps the build above the current stable version and below its possible patch release while making every workflow attempt unique. Workflow tooling is checked out from the workflow revision, and GoReleaser builds the selected commit with trusted release notes but without GitHub credentials.

The normal `mise use -g github:dkarter/hwt` command excludes prereleases by default. Install a development build only through the exact command in its release notes, such as `mise use -g github:dkarter/hwt@0.6.2-dev.20260911.42.1.gabcdef1`. Advanced users can opt into prerelease resolution with `"github:dkarter/hwt" = { version = "latest", prerelease = true }` in their mise TOML configuration.

Repository administrators must enable **Release immutability** in the GitHub repository settings. GitHub applies that setting only to releases published after it is enabled; it does not retroactively lock existing releases. The development workflow uploads every asset and the checksum file to a draft before publication so an enabled policy locks a complete release.

Run the live Herdr plugin lifecycle tier in its isolated Docker server:

```bash
mise run e2e-live
```

The Astro and Starlight site lives in `website/`. Run `mise run website-dev` locally or `mise run website-build` for a production build.

## License

MIT
