# Project Configuration

Use this reference only when building or changing a repository's hwt config.

## Workflow

1. Inspect the repository's tool manifests, ignored local files, dependency
   directories, and setup commands.
2. Generate `.herdr-worktree.yaml` with `hwt config init` if it does not exist.
   Never overwrite an existing config.
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
worktree_dir: ../
worktree_naming: full
worktree_prefix: project-

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

Remove fields the project does not need. Repository scalar values override
global values. Repository `files.copy` and `post_create` replace global lists
unless `<global>` appears where the global entries should be inserted.

## Choosing Settings

- `agent`: Set the command an orchestrator should start in the root pane.
- `worktree_dir`: Resolve relative values from the repository root.
- `worktree_naming`: Use `full` to preserve branch hierarchy in the checkout
  name, or `basename` to use only the final branch component.
- `worktree_prefix`: Add a stable project prefix when checkout names could
  collide.
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

All copy paths must stay within the repository. Prefer setup commands over
copying generated state when regeneration is reliable and reasonably fast.
