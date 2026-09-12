# command-line Specification

## Purpose

Expose a predictable command-line entry point and shell integration.

## Requirements

### Requirement: Discover commands

HWT SHALL expose its supported command tree and flags through contextual help.

#### Scenario: Root and command help {#CLI-001}

- GIVEN an installed `hwt` binary
- WHEN the user runs `hwt --help` or help for a subcommand
- THEN HWT prints applicable commands, arguments, and flags without performing the command

### Requirement: Report the installed version

HWT SHALL report the version embedded in the running binary.

#### Scenario: Version output {#CLI-002}

- GIVEN an HWT binary built with a version
- WHEN the user runs `hwt --version`
- THEN HWT prints that version and exits successfully

### Requirement: Select the Herdr executable

HWT SHALL use `herdr` by default and SHALL allow the executable to be selected globally.

#### Scenario: Herdr binary override {#CLI-003}

- GIVEN a command that invokes Herdr
- WHEN `--herdr-bin PATH` or `HERDR_BIN_PATH` selects another executable
- THEN HWT invokes the selected executable

### Requirement: Generate shell completion

HWT SHALL generate completion scripts for Bash, Zsh, Fish, and PowerShell.

#### Scenario: Supported completion shells {#CLI-004}

- GIVEN one of the supported shell names
- WHEN the user runs `hwt completion SHELL`
- THEN HWT prints a completion script for the current command tree
