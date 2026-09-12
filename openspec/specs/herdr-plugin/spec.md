# herdr-plugin Specification

## Purpose

Install the official plugin and expose safe interactive Herdr actions and events.

## Requirements

### Requirement: Manage the official plugin

HWT SHALL delegate installation, update, and removal of plugin `hwt.worktrees` to Herdr.

#### Scenario: Install released plugin {#HDR-001}

- GIVEN a released HWT binary
- WHEN the user runs `hwt plugin install`
- THEN Herdr installs `dkarter/hwt/plugins/herdr` from the matching release tag with confirmation pre-approved

#### Scenario: Update or uninstall plugin {#HDR-002}

- GIVEN the official plugin is installed
- WHEN the user runs `hwt plugin update` or `hwt plugin uninstall`
- THEN Herdr atomically refreshes the source or removes plugin `hwt.worktrees` respectively

#### Scenario: Development plugin source {#HDR-003}

- GIVEN an HWT binary without a release or development-version tag
- WHEN plugin installation or update runs
- THEN HWT allows Herdr to use the repository's default source revision

### Requirement: Prepare Herdr-created worktrees

The plugin SHALL react to `worktree.created` by applying HWT copy and environment preparation once.

#### Scenario: Worktree-created event {#HDR-004}

- GIVEN Herdr creates a linked worktree through its UI or CLI
- WHEN the plugin receives `worktree.created`
- THEN it runs `hwt copy` for the event worktree
- AND duplicate events remain safe

### Requirement: Create through a native action

The plugin SHALL provide `hwt.worktrees.new` as an interactive workspace action.

#### Scenario: Interactive create action {#HDR-005}

- GIVEN a Herdr workspace in a Git repository
- WHEN the user invokes New configured worktree
- THEN HWT prompts for and normalizes a branch name, offers local base branches, creates the configured worktree, and focuses it by default

#### Scenario: Cancel interactive creation {#HDR-006}

- GIVEN an interactive branch or base prompt
- WHEN the user cancels or submits no branch
- THEN HWT exits without creating a worktree

### Requirement: Remove through a native action

The plugin SHALL provide `hwt.worktrees.remove` with destructive confirmation defaulting to No.

#### Scenario: Interactive safe removal {#HDR-007}

- GIVEN the current workspace is a linked worktree
- WHEN the user confirms Remove current worktree
- THEN HWT performs normal safe removal

#### Scenario: Separate force confirmation {#HDR-008}

- GIVEN normal interactive removal finds a dirty or locked worktree
- WHEN HWT offers the applicable force warning
- THEN removal proceeds only after a second explicit confirmation
