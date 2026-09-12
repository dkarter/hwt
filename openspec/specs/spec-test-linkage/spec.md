# spec-test-linkage Specification

## Purpose

Keep executable tests bidirectionally linked to stable OpenSpec scenarios.

## Requirements

### Requirement: Validate scenario links

HWT SHALL provide validation that rejects missing scenario IDs, duplicate IDs, untagged tests, unknown test tags, and scenarios without tests.

#### Scenario: Repository spec links are complete {#META-001}

- GIVEN active OpenSpec scenario headings and executable HWT tests
- WHEN the spec-link validator runs
- THEN every scenario ID is globally unique and has a valid bidirectional test link

### Requirement: Run focused scenario tests

HWT SHALL provide one focused runner that resolves a scenario ID, capability name, or spec path to its linked tests.

#### Scenario: Focused runner resolves linked tests {#META-002}

- GIVEN a valid scenario ID, capability name, or capability spec path
- WHEN the focused spec runner resolves it
- THEN it runs or emits the corresponding test filters for every selected scenario
