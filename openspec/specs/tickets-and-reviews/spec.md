# tickets-and-reviews Specification

## Purpose

Create ticket-backed worktrees, open pull requests, and establish exact review workspaces.

## Requirements

### Requirement: Derive or accept a branch name

HWT SHALL normalize whitespace in a positional title to hyphens unless `--ticket` requests an optional named ticket command.

#### Scenario: Ticket creates a branch {#REV-001}

- GIVEN `hwt create --ticket=NAME INPUT` and a configured named ticket command
- WHEN `{input}` is replaced in argv and the command returns JSON matching its output selectors
- THEN HWT creates the mapped branch through the normal worktree lifecycle
- AND stores mapped string metadata as `ticket.*`

#### Scenario: Invalid ticket response {#REV-002}

- GIVEN the selected ticket command fails, emits invalid JSON, lacks a mapped string branch, or returns conflicting branch or path identity
- WHEN command-backed branch creation runs
- THEN HWT reports the problem and does not ask Herdr to create that worktree

#### Scenario: Positional title becomes a branch by default {#REV-008}

- GIVEN no selected ticket command
- WHEN the user runs `hwt create 'investigation cache'`
- THEN HWT creates `investigation-cache` without running an external ticket command

#### Scenario: Ticket integration remains opt-in {#REV-009}

- GIVEN configured ticket commands
- WHEN the user runs `hwt create TITLE` without `--ticket`
- THEN HWT creates a branch from the normalized title without running a ticket command

#### Scenario: Select an existing ticket {#REV-010}

- GIVEN `ticket_commands.default` supports interactive selection and its final argument is `{input}`
- WHEN the user runs `hwt create --ticket` without a query
- THEN HWT omits that argument, connects terminal input and stderr for the picker, and uses its mapped branch from stdout JSON

### Requirement: Open a branch pull request

HWT SHALL provide an overridable `urls.pr` default that resolves pull requests through authenticated GitHub CLI.

#### Scenario: Open or print pull request {#REV-003}

- GIVEN a current or explicit branch with a GitHub pull request
- WHEN the user runs `hwt url pr [branch]`
- THEN HWT prints the pull request URL
- AND `--open` opens it in the platform browser

### Requirement: Resolve review targets without changing the source checkout

HWT SHALL accept one full HTTPS GitHub pull request URL, positive decimal pull request number, or local, remote-qualified, or unambiguous remote branch.

#### Scenario: Review a pull request including a fork {#REV-004}

- GIVEN a valid pull request URL or number and an identifiable base-repository remote
- WHEN the user runs `hwt review URL_OR_NUMBER`
- THEN HWT fetches and verifies the exact pull request head and base commits without changing the primary checkout
- AND fork heads are supported through the base repository pull-request ref

#### Scenario: Review a branch {#REV-005}

- GIVEN a local branch, `REMOTE/BRANCH`, or an unfetched branch with one eligible remote
- WHEN the user runs `hwt review SELECTOR`
- THEN HWT resolves its exact commit and creates a deterministic HWT-managed review branch
- AND ambiguous remote selection fails until `--remote` or a qualified selector is supplied

### Requirement: Reuse only an exact review workspace

HWT SHALL reuse a review worktree only when its managed branch, head commit, and pinned base match the request.

#### Scenario: Exact review reuse {#REV-006}

- GIVEN an exact existing review worktree
- WHEN the same target is reviewed again
- THEN HWT reopens a closed workspace or reuses the open workspace
- AND it does not start a duplicate while the recorded review pane is busy
- AND it relaunches the reviewer when that pane is idle

### Requirement: Preserve recoverable review workspaces

HWT SHALL launch the configured review argv in the review checkout and SHALL preserve a successfully created workspace when launch fails.

#### Scenario: Review launch result {#REV-007}

- GIVEN a pull request review target and a created or reused review workspace
- WHEN HWT launches `review_command`
- THEN an exact `{pr_url}` argument expands to the canonical pull request URL and other configured arguments remain literal
- AND JSON reports identity, commit, branch, path, workspace and pane IDs, reuse, command, status, and any recoverable launch error

#### Scenario: Reject pull request URL placeholder for a branch {#REV-011}

- GIVEN a branch review target and a `review_command` containing an exact `{pr_url}` argument
- WHEN HWT validates the review target
- THEN HWT fails before fetching the branch or creating a review branch or workspace
