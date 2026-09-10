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
hwt create 'describe the work to do'
hwt create --branch feature/name --base main --json
hwt copy
hwt env
hwt env --json
hwt env --refresh
hwt env -- bin/dev
hwt list
hwt preview
hwt preview feature/name --json
hwt pr
hwt pr feature/name --json
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

`hwt create DESCRIPTION` runs `lnr quick --json DESCRIPTION`, reads its `branchName`, and creates that branch through the normal Herdr flow. The description is passed as one process argument without a shell. Use `hwt create --branch BRANCH` for explicit branch creation; a description and `--branch` are mutually exclusive. Creation defaults to the current Git branch as its base and does not change focus. Its JSON result includes the workspace ID, root pane ID, checkout path, base branch, copied files, and configured agent command.

`hwt copy` copies configured files from the primary checkout into the current linked worktree once. It reads Herdr plugin event context automatically, and concurrent or repeated calls are safe no-ops.

`hwt env` creates or refreshes the ignored `.env.worktree` file and reports its path. Pass `--json` to inspect values, `--refresh` to replace allocated ports, or `-- COMMAND...` to run a development command with the variables in its process environment.

`hwt remove` refuses dirty or locked worktrees unless `--force` is provided. It quickly renames the checkout out of the way, closes the Herdr workspace, removes Git's worktree metadata, and deletes the checkout in the background.

`hwt pr [branch]` resolves the branch's pull request through the authenticated GitHub CLI and opens it in the default browser. It uses the current branch when omitted; `--json` prints `{"url":"..."}` without opening a browser. Repositories with multiple remotes require `--repo [HOST/]OWNER/REPO`, which also supports pull requests from forks.

`hwt preview [branch]` expands the configured `preview_url` and opens it in the default browser. It uses the current branch and worktree when omitted; `--json` prints `{"url":"..."}` without opening a browser. Explicit branches need not exist locally, but cannot use the `{worktree}` placeholder because they are not associated with the current checkout.

`hwt skill` prints the canonical usage skill for AI agents. Its concise core points agents to `hwt skill config`, which prints the project-configuration reference only when needed.

## Configuration

Global defaults live at `${XDG_CONFIG_HOME:-~/.config}/hwt/config.yaml`. A repository may override them with `.herdr-worktree.yaml` or `.herdr-worktree.yml` at its root. When neither project file exists, hwt falls back to `<git-common-dir>/hwt/config.yaml` or `.yml`, which is machine-local and shared by every linked worktree.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json
agent: opencode --port
ticket_command: [lnr, quick, --json]
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-
preview_url: https://{sanitized_branch}.preview.example.com

ports:
  start: 20000
  end: 39999
  services: [web, assets]

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

Project values override global values. `ticket_command` is an argv array and is replaced as a whole; hwt appends the task description as one final argument and requires JSON output containing a non-empty string `branchName`. Other project lists replace global lists unless they contain `<global>` at the position where global entries should be inserted. This includes `ports.services`; `environment.variables` replaces the global map as a unit. Missing copy sources are ignored. Copy paths must remain within the repository.

`preview_url` is a scalar and supports `{repository}`, `{branch}`, `{sanitized_branch}`, and `{worktree}`. Repository is the primary checkout directory name; worktree is the current checkout directory name and is available only when the branch argument is omitted. Sanitization lowercases ASCII letters, replaces each run of characters outside `a-z0-9` with `-`, removes leading or trailing separators, and limits the result to 63 characters. HWT UTF-8 percent-encodes every substituted value except RFC 3986 unreserved characters (`A-Z`, `a-z`, `0-9`, `-._~`), so use `{sanitized_branch}` for a hostname label and any placeholder in a path or query component. Unknown, malformed, unavailable, or non-HTTP(S) templates are rejected before a browser opens.

HWT reserves stable, collision-free ports in `${XDG_STATE_HOME:-~/.local/state}/hwt/ports.json` under a process lock. It writes `HWT_PORT_WEB`, `HWT_PORT_ASSETS`, the worktree path and branch, and configured non-secret variables to a mode-`0600` `.env.worktree`. The generated file is added to Git's repository-local exclude file and is available to `post_create`; see the [environment design](website/src/content/docs/worktree-environment.md) for lifecycle, security, mise, Herdr, and reverse-proxy details.

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

Releases are managed by Release Please and GoReleaser. CI tests on macOS and Linux and builds the CLI on every pull request.

Run the live Herdr plugin lifecycle test from a Herdr-managed pane:

```bash
./scripts/test-herdr-plugin-e2e.sh
```

The Astro and Starlight site lives in `website/`. Run `mise run website-dev` locally or `mise run website-build` for a production build.

## License

MIT
