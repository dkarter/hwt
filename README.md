# hwt

`hwt` is a deterministic wrapper around Herdr's native Git worktree lifecycle. It applies global and repository-specific placement, file-copy, and setup rules before returning workspace and pane IDs to people or coding agents.

## Install

```bash
mise use -g github:dkarter/hwt
```

Build from source with `mise run build` or install to `~/.local/bin/hwt` with `mise run install`.

## Commands

```bash
hwt create --branch feature/name --base main --json
hwt list
hwt remove --workspace w1A --json
hwt config show
hwt config validate
hwt config init
hwt config init --global
hwt schema
hwt completion zsh
```

`hwt create` defaults to the current Git branch as its base and creates the Herdr workspace without changing focus. Its JSON result includes the workspace ID, root pane ID, checkout path, base branch, copied files, and configured agent command.

`hwt remove` refuses dirty or locked worktrees unless `--force` is provided. It quickly renames the checkout out of the way, closes the Herdr workspace, removes Git's worktree metadata, and deletes the checkout in the background.

## Configuration

Global defaults live at `${XDG_CONFIG_HOME:-~/.config}/hwt/config.yaml`. A repository may override them with `.herdr-worktree.yaml` or `.herdr-worktree.yml` at its root.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json
agent: opencode --port
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-

files:
  copy:
    - <global>
    - .env.local

post_create:
  - <global>
  - mise install
```

Project scalar values override global values. Project lists replace global lists unless they contain `<global>` at the position where global entries should be inserted. Missing copy sources are ignored. Copy paths must remain within the repository.

The schema is published at `schema/herdr-worktree.schema.json` and embedded in each binary. `hwt schema` prints the exact embedded schema, while `hwt config validate` performs strict YAML and semantic validation.

## Development

```bash
mise install
mise run check
mise run snapshot
```

Releases are managed by Release Please and GoReleaser. CI tests on macOS and Linux and builds the CLI on every pull request.

## License

MIT
