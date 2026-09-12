# environment-and-local-dns Specification

## Purpose

Provide stable per-worktree ports, generated environments, and opt-in local routes.

## Requirements

### Requirement: Allocate stable worktree ports

HWT SHALL assign each configured service an available port in the inclusive configured range and preserve valid allocations across repeated runs.

#### Scenario: Stable concurrent allocation {#ENV-001}

- GIVEN multiple linked worktrees request named services
- WHEN their environments are generated concurrently or repeatedly
- THEN each worktree receives non-conflicting available ports
- AND an existing valid allocation remains stable

#### Scenario: Refresh allocations {#ENV-002}

- GIVEN a worktree with allocated ports
- WHEN the user runs `hwt env --refresh`
- THEN HWT releases and reallocates the ports for that worktree and updates dependent local routes
- AND the resulting ports remain valid and non-conflicting, even when the same numbers remain available

### Requirement: Generate a private environment file

HWT SHALL write `.env.worktree` with generated identity and port variables plus configured non-secret values.

#### Scenario: Generate and inspect environment {#ENV-003}

- GIVEN a linked worktree and valid environment configuration
- WHEN the user runs `hwt env`
- THEN HWT writes a mode-`0600` ignored dotenv file and prints its path
- AND `--json` returns the path and complete generated variable map

#### Scenario: Run with environment {#ENV-004}

- GIVEN `hwt env -- COMMAND...`
- WHEN environment generation succeeds
- THEN HWT executes the command with the generated variables and returns its outcome

#### Scenario: Protect environment boundaries {#ENV-005}

- GIVEN `.env.worktree` is tracked, requested as a copy source, or configured values reference unknown generated variables
- WHEN HWT prepares the environment
- THEN HWT rejects the unsafe configuration or overwrite
- AND it does not import parent or other dotenv values while expanding configuration

### Requirement: Manage HWT-owned local DNS state

HWT SHALL generate and report only HWT-owned dnsmasq, Caddy, and route-registry files for enabled local DNS.

#### Scenario: Set up and inspect local DNS {#ENV-006}

- GIVEN valid `local_dns` and at least one port service on macOS or Linux
- WHEN the user runs `hwt dns setup` and `hwt dns status`
- THEN HWT reports generated snippet paths and registered exact service routes in plain or JSON output
- AND HWT does not install services, edit system configuration, invoke `sudo`, or manage listeners

#### Scenario: Register and refresh routes {#ENV-007}

- GIVEN local DNS is enabled for a linked worktree
- WHEN create, copy, `hwt env`, or `hwt dns refresh` prepares it
- THEN HWT registers a stable collision-resistant base hostname and per-service HTTP URLs
- AND refresh reconciles routes without replacing ports

#### Scenario: Tear down local DNS {#ENV-008}

- GIVEN HWT-owned local DNS state
- WHEN the user runs `hwt dns teardown`
- THEN HWT refuses while routes are registered unless `--force` is explicit
- AND successful teardown removes only HWT-owned generated state
- AND failed configured reloads restore the previous generated state
