# named-urls-and-metadata Specification

## Purpose

Resolve branch-aware URLs from safe built-in, ticket, static, and lazy metadata.

## Requirements

### Requirement: Resolve named URLs

HWT SHALL resolve `urls.NAME` using the current worktree or an explicit branch and print the result without opening it by default.

#### Scenario: Plain and JSON URL output {#URL-001}

- GIVEN a configured named absolute URL with an optional display label
- WHEN the user runs `hwt url NAME [branch]`
- THEN HWT prints the resolved URL
- AND `--json` prints its name and URL as JSON
- AND includes the label only when configured

#### Scenario: List all URLs as JSON {#URL-008}

- GIVEN named URLs for the current branch
- WHEN the user runs `hwt url --json`
- THEN HWT prints every available name and computed URL in deterministic order
- AND includes each configured display label
- AND omits URLs that require pull request metadata when the branch has no pull request
- AND includes the default repository URL when GitHub can identify the repository
- AND fails on other URL resolution errors

#### Scenario: Complete configured URL names {#URL-009}

- GIVEN URL names from defaults and merged user configuration
- WHEN the shell requests completion after `hwt url`
- THEN HWT offers the URL names for the current Git repository

#### Scenario: Explicit browser opening {#URL-002}

- GIVEN a resolved HTTP or HTTPS named URL
- WHEN the user supplies `--open`
- THEN HWT opens it in the platform browser
- AND non-browser schemes are rejected for opening but remain printable

### Requirement: Define URL commands through configuration

HWT SHALL accept each configured URL as a template string or an object with exactly one of `template` or `service` and an optional display `label`, expose configured URL names only through `hwt url`, and provide overridable GitHub repository and pull request URLs named `repo` and `pr` by default. A global or repository URL entry set to `false` SHALL disable the inherited entry, and a more specific URL definition SHALL re-enable it.

#### Scenario: Preview output modes {#URL-003}

- GIVEN `urls.preview` can be resolved
- WHEN the user runs `hwt url preview [branch]`
- THEN HWT prints its computed URL without waiting for deployment readiness
- AND `--open` opens it in the platform browser

#### Scenario: Generated service URL {#URL-010}

- GIVEN a named URL references a configured port service
- WHEN the user runs `hwt url NAME` from a linked worktree
- THEN HWT allocates or reuses that worktree's service port
- AND prints the generated service URL verbatim
- AND the service URL is unavailable with an explicit branch

### Requirement: Cache named URLs

HWT SHALL support optional persistent caching on each named template URL, cache the default GitHub `pr` and `repo` URLs, use a shorter lifetime for missing pull requests, and bypass and update caches when `--refresh` is supplied.

#### Scenario: Cache and refresh command-backed URL {#URL-011}

- GIVEN a named URL with caching enabled and command-backed metadata
- WHEN the URL is resolved repeatedly
- THEN HWT reuses the cached result until it expires
- AND `--refresh` resolves and stores a fresh result
- AND `cache: false` disables caching for an overridden default URL

### Requirement: Expand built-in placeholders safely

HWT SHALL provide `repository`, `branch`, `sanitized_branch`, `worktree`, `hostname`, lazily resolved GitHub repository host, owner, and name values, and lazily resolved pull request host, owner, repository, and number values where available.

#### Scenario: Sanitize and encode substitutions {#URL-004}

- GIVEN a URL template with available placeholders
- WHEN HWT expands it
- THEN branch sanitization yields a lowercase DNS-safe value of at most 63 characters
- AND every substituted value is UTF-8 percent-encoded except RFC 3986 unreserved characters

#### Scenario: Unavailable placeholder {#URL-005}

- GIVEN an explicit branch with a template requiring worktree-local `worktree`, `hostname`, or `ticket.*`, or any unknown value
- WHEN HWT resolves the URL
- THEN resolution fails before printing or opening a URL

### Requirement: Resolve metadata from isolated namespaces

HWT SHALL resolve static values, persisted ticket values, command namespaces, and reserved built-ins while preventing configured metadata from shadowing reserved names.

#### Scenario: Metadata namespaces and persistence {#URL-006}

- GIVEN static and persisted ticket metadata for a checked-out worktree
- WHEN a named URL is resolved
- THEN each value resolves through its documented namespace and reserved built-ins remain authoritative
- AND ticket metadata is private per-worktree state while command output and resolved URLs are not persisted

### Requirement: Run command metadata lazily and literally

HWT SHALL run only metadata command namespaces requested by a template, directly as configured argv and without ambient environment expansion.

#### Scenario: Lazy metadata command {#URL-007}

- GIVEN a URL requests `{database.host}` backed by `metadata.commands.database`
- WHEN HWT resolves that URL
- THEN it substitutes supported built-ins into individual argv entries and runs only that command
- AND the command must return one JSON object whose keys are valid identifiers and whose values are strings
