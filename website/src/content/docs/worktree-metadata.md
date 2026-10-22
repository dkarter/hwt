---
title: Worktree metadata
description: Use static, ticket, and lazy command metadata in worktree URLs.
---

Metadata adds namespaced placeholders to [worktree URL](../worktree-urls/)
templates. HWT supports static values, metadata saved while creating a worktree
from a ticket, and command output resolved only when requested.

## Static metadata

Configure non-secret values under `metadata.values`:

```yaml
metadata:
  values:
    region: us-east-1

urls:
  dashboard: https://dashboard.example.com/{region}/{sanitized_branch}
```

Global and repository values merge by name, with repository entries winning. Do
not put credentials in tracked static metadata.

## Ticket metadata

A ticket command can map string fields from its JSON output into private
worktree metadata:

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
```

After `hwt create --ticket`, those fields are available as
`{ticket.identifier}`, `{ticket.title}`, and `{ticket.url}`. A ticket command
without output mappings may instead return a dedicated string-valued `metadata`
object alongside `branchName`. HWT ignores arbitrary top-level fields.

Ticket metadata belongs to a specific worktree. It is unavailable when resolving
a URL for an explicit branch.

### Lifecycle hook environment

`post_create`, `pre_remove`, and `post_remove` receive these hook-only variables:

| Variable                | Persisted ticket metadata field |
| ----------------------- | ------------------------------- |
| `HWT_TICKET_IDENTIFIER` | `identifier`                    |
| `HWT_TICKET_TITLE`      | `title`                         |
| `HWT_TICKET_URL`        | `url`                           |

All three variables are set, with an empty string for a missing field or an
unassigned worktree. Values are passed literally, not evaluated as shell code.
HWT strips inherited `HWT_TICKET_*` variables before adding this worktree's
values. Other ticket metadata fields are not exported. No ticket is inferred
from a branch name, and hooks do not run ticket commands or metadata commands.

Both synchronous and async removal hooks use the same snapshot, captured before
`pre_remove` and retained after Git metadata deletion for `post_remove`. These
variables are not added to `.env.worktree` or `hwt env` output.

A global cleanup hook can skip unassigned worktrees without parsing branches:

```yaml
pre_remove:
  - 'if [ -z "$HWT_TICKET_IDENTIFIER" ]; then exit 0; fi; ticket-cleanup "$HWT_TICKET_IDENTIFIER"'
```

`ticket-cleanup` is a user-provided command; HWT does not integrate with a
specific ticket provider.

See [configuration](../configuration/#ticket_commands) for ticket command input,
output selectors, and command execution rules.

## Lazy command metadata

Use a command namespace for values that should be fetched at resolution time:

```yaml
metadata:
  commands:
    database: [bin/database-metadata, --branch, '{branch}', --json]

urls:
  database: postgres://{database.user}:{database.password}@{database.host}/app
```

HWT runs `metadata.commands.database` only when a template requests a
`{database.*}` value. It invokes the configured argv directly without a shell.
The command must return one JSON object whose values are strings.

Built-in placeholders in command arguments (`repository`, `branch`,
`sanitized_branch`, `worktree`, and `hostname`) are substituted as one argument.
Custom metadata, `pr_number`, and ambient environment variables are not expanded.

Use lazy commands for database passwords, tokens, and other secrets. HWT does
not persist command output or resolved URLs.

## Resolution and storage

Names resolve in this order: static values, `ticket.*`, command namespaces, then
reserved built-ins. Unknown or missing placeholders fail URL resolution.
Configured metadata cannot shadow reserved names.

Ticket metadata is stored in a mode-`0600` file under private Git worktree
metadata and removed with the worktree. It is not written to the checkout.
