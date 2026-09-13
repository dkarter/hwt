# Development

## Setup

Install the toolchain and run all checks:

```bash
mise install
mise run check
```

Build the CLI with `mise run build`, install it to `~/.local/bin/hwt` with `mise run install`, or build release artifacts locally with `mise run snapshot`.

Product behavior is specified under `openspec/specs/` and linked to compiled-binary E2E tests by scenario IDs. See the [testing guide](docs/testing.md) for adding scenarios, running one linked test, and diagnosing hermetic or live Herdr failures.

Run the live Herdr plugin lifecycle tier in its isolated Docker server:

```bash
mise run e2e-live
```

When developing the Herdr plugin locally, use:

```bash
herdr plugin link ./plugins/herdr
```

## Website

The Astro and Starlight site lives in `website/`. Run `mise run website-dev` locally or `mise run website-build` for a production build.

## Releases

Releases are managed by Release Please and GoReleaser. CI tests on macOS and Linux and builds the CLI on every pull request. Every push to `main` publishes an immutable development prerelease, and maintainers can run the **Publish development build** workflow for another Git ref.

Development versions use the next patch after the highest published stable SemVer release, followed by `-dev.YYYYMMDD.RUN.ATTEMPT.gSHA`. For example, stable `v0.6.1` produces `v0.6.2-dev.20260911.42.1.gabcdef1`. This keeps the build above the current stable version and below its possible patch release while making every workflow attempt unique. Workflow tooling is checked out from the workflow revision, and GoReleaser builds the selected commit with trusted release notes but without GitHub credentials.

The normal `mise use -g github:dkarter/hwt` command excludes prereleases by default. Install a development build only through the exact command in its release notes, such as `mise use -g github:dkarter/hwt@0.6.2-dev.20260911.42.1.gabcdef1`. Advanced users can opt into prerelease resolution with `"github:dkarter/hwt" = { version = "latest", prerelease = true }` in their mise TOML configuration.

Repository administrators must enable **Release immutability** in the GitHub repository settings. GitHub applies that setting only to releases published after it is enabled; it does not retroactively lock existing releases. The development workflow uploads every asset and the checksum file to a draft before publication so an enabled policy locks a complete release.
