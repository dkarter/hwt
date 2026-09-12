# distribution-and-integration Specification

## Purpose

Publish matching schemas, agent guidance, plugins, and development builds.

## Requirements

### Requirement: Print the binary's configuration schema

HWT SHALL embed and print the JSON Schema that matches the running binary.

#### Scenario: Schema output {#DIST-001}

- GIVEN an installed HWT binary
- WHEN the user runs `hwt schema`
- THEN HWT prints a valid JSON Schema describing supported strict configuration

### Requirement: Publish agent guidance

HWT SHALL embed concise canonical agent instructions and a separate on-demand configuration reference.

#### Scenario: Skill output {#DIST-002}

- GIVEN an installed HWT binary
- WHEN the user runs `hwt skill` or `hwt skill config`
- THEN HWT prints the matching usage skill or project-configuration reference respectively

### Requirement: Package integration assets

HWT SHALL distribute the configuration schema and official Herdr plugin alongside supported release archives.

#### Scenario: Release integration assets {#DIST-003}

- GIVEN a published HWT release
- WHEN a user obtains its archive or installs its plugin
- THEN the matching schema and plugin assets are available for that release

### Requirement: Version development releases uniquely

HWT SHALL derive a development prerelease from the next patch after the highest published stable semantic version.

#### Scenario: Development version format {#DIST-004}

- GIVEN stable version `MAJOR.MINOR.PATCH`, a UTC date, positive run and attempt numbers, and a commit SHA
- WHEN a development version is resolved
- THEN it is `vMAJOR.MINOR.(PATCH+1)-dev.YYYYMMDD.RUN.ATTEMPT.gSHA7`
- AND invalid version inputs are rejected

### Requirement: Publish complete immutable-ready prereleases

The development release workflow SHALL build with normal packaging, upload all assets and checksums to a draft, and publish it as a non-latest prerelease only after quality checks pass.

#### Scenario: Development release publication {#DIST-005}

- GIVEN a manual dispatch for a Git ref and no conflicting development tag
- WHEN quality, version, and packaging checks succeed
- THEN the workflow publishes a complete prerelease with exact-install instructions
- AND failure before publication cleans up the draft and tag
