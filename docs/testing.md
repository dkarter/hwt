# Specifications and end-to-end tests

OpenSpec is the source of truth for hwt's user-visible commands and lifecycle behavior. Active
capability specifications live under `openspec/specs/<capability>/spec.md`. The default E2E suite
builds the real `hwt` executable and runs it against disposable Git repositories, XDG directories,
and fake external commands.

## Add or change behavior

1. Update the relevant capability spec before changing behavior. Requirements use normative `SHALL`
   language and scenarios use `GIVEN`, `WHEN`, and `THEN` steps.
2. Give each new scenario a permanent, globally unique ID such as `{#WT-013}`. Use a two-to-eight
   letter capability prefix and three digits. Never renumber or reuse an ID after publication.
3. Add a public-boundary test under `e2e/`. Encode every covered ID in the top-level Go test name by
   removing the hyphen, for example `TestWT013CreatesConfiguredWorktree`. A test can cover multiple
   scenarios, and a scenario can be covered by multiple tests.
4. Run `mise run check` before submitting the change. Use `mise run spec-check` or `mise run e2e` when
   iterating on only one layer.

The link validator rejects scenarios without IDs, malformed or duplicate IDs, unknown test IDs,
E2E tests without links, and scenarios without tests. Archived change specifications, if present,
are outside `openspec/specs` and do not count as active product behavior.

## Focus a specification

The focused runner accepts exactly one selector:

```bash
mise run test-spec -- --scenario WT-013
mise run test-spec -- --capability worktree-lifecycle
mise run test-spec -- --spec worktree-lifecycle/spec.md
```

Add `--list` to inspect the matching test names without running them. `mise run spec-check` first runs
the official strict OpenSpec validator and then checks bidirectional test links.

## Test tiers

- `mise run e2e` is hermetic and runs in normal CI on macOS and Linux. It does not read normal user
  configuration, inherit credentials, contact services, alter system DNS, or open a real browser.
- `mise run e2e-live` is platform-dependent and runs only in Docker. The image starts a named
  `hwt-e2e` Herdr server with temporary `HERDR_CONFIG_PATH`, HOME, and XDG roots. It installs the
  local plugin only into that isolated server, creates disposable workspaces, and runs the container
  without network access. The lifecycle script fails closed unless every isolation marker and path
  points below `/tmp/hwt-herdr-e2e`.
- `mise run check` includes unit tests, hermetic E2E, strict OpenSpec and link validation, formatting,
  release validation, the website build, and vetting. It intentionally excludes the live tier.

## Diagnose failures

Hermetic test failures retain the failing command's stdout and stderr in the Go test output. Re-run a
single scenario with the focused runner, or use `go test ./e2e -run TestName -count=1 -v`. A missing
fake command generally means the test tried an undeclared integration; add the fake to that test's
sandbox rather than exposing the developer's `PATH`.

The only supported command is `mise run e2e-live`; never invoke the inner lifecycle script directly.
The Docker runner does not mount the host Herdr configuration, sockets, plugin storage, or source
checkout. Install, update, and uninstall command behavior is also covered in the hermetic suite with
a fake Herdr executable.

For a live failure, confirm the Docker daemon is available. The runner removes its container and
image on exit. Re-run with `mise run e2e-live`; do not weaken the isolation checks or mount host Herdr
paths to diagnose it.
