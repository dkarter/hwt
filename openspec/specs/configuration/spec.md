# configuration Specification

## Purpose

Locate, compose, initialize, inspect, and validate HWT policy.

## Requirements

### Requirement: Resolve configuration scopes

HWT SHALL combine global configuration with repository policy, preferring a checkout config over shared Git-local policy.

#### Scenario: Configuration precedence {#CFG-001}

- GIVEN global configuration and repository configuration
- WHEN HWT resolves configuration for a repository
- THEN repository scalar and whole-value fields override global values
- AND mergeable maps merge by name with repository entries winning

#### Scenario: Global list insertion {#CFG-002}

- GIVEN a repository list containing `<global>`
- WHEN HWT resolves `files.copy`, `ports.services`, or `post_create`
- THEN global entries appear at the marker position
- AND `<global>` in global configuration is rejected

### Requirement: Report configuration paths

HWT SHALL report the effective project, shared Git-local, or XDG global configuration path.

#### Scenario: Select a configuration path {#CFG-003}

- GIVEN a repository and optional `--global` or `--git-common`
- WHEN the user runs `hwt config path`
- THEN HWT prints the selected path
- AND mutually exclusive scope flags cannot be combined

### Requirement: Initialize configuration safely

HWT SHALL create a schema-linked starter configuration and SHALL not overwrite an existing file.

#### Scenario: Initialize a selected scope {#CFG-004}

- GIVEN a selected project, Git-local, or global scope without a config file
- WHEN the user runs `hwt config init` for that scope
- THEN HWT creates and prints the starter file path

#### Scenario: Refuse an existing file {#CFG-005}

- GIVEN the selected config file already exists
- WHEN the user runs `hwt config init`
- THEN HWT fails without changing that file

### Requirement: Inspect resolved configuration

HWT SHALL expose the resolved configuration and contributing source paths as JSON.

#### Scenario: Show resolved configuration {#CFG-006}

- GIVEN valid configuration for the current repository
- WHEN the user runs `hwt config show`
- THEN HWT prints the resolved values and source paths as one JSON result

### Requirement: Validate configuration strictly

HWT SHALL reject unknown YAML fields, malformed values, unsafe paths, invalid templates, incompatible options, and multiple YAML documents.

#### Scenario: Validate a file or repository {#CFG-007}

- GIVEN either an explicit config path or the current repository configuration
- WHEN the user runs `hwt config validate`
- THEN valid configuration succeeds with confirmation
- AND invalid configuration fails with a diagnostic identifying the violated contract
