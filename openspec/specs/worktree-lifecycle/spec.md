# worktree-lifecycle Specification

## Purpose

Create, prepare, list, and safely remove Herdr-managed linked worktrees.

## Requirements

### Requirement: Create an explicit branch worktree

HWT SHALL create a Herdr workspace from exactly one free-form title, explicit branch, or ticket input.

#### Scenario: Explicit branch creation {#WT-001}

- GIVEN a valid repository, branch, and base commit
- WHEN the user runs `hwt create --branch BRANCH`
- THEN HWT creates a linked worktree and Herdr workspace from the selected base
- AND the new workspace is not focused unless `--focus` is supplied

#### Scenario: Creation arguments are exclusive {#WT-002}

- GIVEN neither a title nor `--branch`, or both forms together
- WHEN the user runs `hwt create`
- THEN HWT rejects the invocation before creating a worktree

#### Scenario: Detached base requires selection {#WT-003}

- GIVEN the source checkout has detached HEAD
- WHEN creation omits `--base`
- THEN HWT refuses creation and asks for an explicit base ref

### Requirement: Honor worktree placement

HWT SHALL apply configured directory, naming mode, prefix, and explicit path overrides to new checkouts.

#### Scenario: Configured and explicit paths {#WT-004}

- GIVEN worktree placement configuration or `--path`
- WHEN a worktree is created
- THEN HWT uses the explicit path when present
- AND otherwise derives a safe checkout name according to the configured naming policy

### Requirement: Prepare a created worktree

HWT SHALL prepare configured files, environment, post-create commands, and base metadata before reporting successful creation.

#### Scenario: Successful preparation order {#WT-005}

- GIVEN configured copy entries, environment values, and post-create commands
- WHEN HWT creates a worktree
- THEN files are prepared before the environment and ordered post-create commands
- AND the selected base is recorded for later lifecycle operations

#### Scenario: Failed preparation rolls back {#WT-006}

- GIVEN Herdr created a worktree but subsequent preparation fails
- WHEN HWT handles the failure
- THEN HWT removes the partial workspace, checkout, port allocation, and local DNS registration

#### Scenario: Structured create result {#WT-007}

- GIVEN successful creation with `--json`
- WHEN HWT completes preparation
- THEN it reports workspace and pane IDs, path, branch, base, agent, copied paths, environment, and configuration sources as JSON

### Requirement: Copy configured files once

HWT SHALL prepare configured paths only once per linked worktree, ignoring missing sources and rejecting paths outside repository boundaries.

#### Scenario: Copy configured entries {#WT-008}

- GIVEN a linked worktree with configured regular-copy, copy-on-write, or symlink entries
- WHEN the user or plugin runs `hwt copy`
- THEN existing sources are transferred with their selected strategy
- AND the worktree environment is generated

#### Scenario: Repeated or concurrent copy {#WT-009}

- GIVEN preparation already completed or another copy is in progress
- WHEN `hwt copy` runs again
- THEN HWT waits as needed and returns the original result without duplicating preparation

### Requirement: List Herdr worktrees

HWT SHALL return Herdr's JSON worktree listing for the repository selected by `--cwd` or the current directory.

#### Scenario: List repository worktrees {#WT-010}

- GIVEN a repository known to Herdr
- WHEN the user runs `hwt list`
- THEN HWT prints the repository's machine-readable worktree list

### Requirement: Remove worktrees safely and quickly

HWT SHALL remove only linked Herdr worktrees and SHALL protect dirty or locked worktrees unless force is explicit.

#### Scenario: Safe removal {#WT-011}

- GIVEN a clean unlocked linked worktree selected by current workspace or `--workspace`
- WHEN the user runs `hwt remove` or `hwt rm`
- THEN HWT closes its workspace, unregisters Git metadata, releases ports and routes, and returns without waiting for bulk deletion

#### Scenario: Protected and forced removal {#WT-012}

- GIVEN a dirty or locked linked worktree
- WHEN removal runs without `--force`
- THEN HWT refuses to discard it
- AND when `--force` is explicit HWT proceeds and reports the removed workspace and path
