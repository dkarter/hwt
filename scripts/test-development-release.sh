#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version_script="$root/scripts/development-version.sh"
workflow="$root/.github/workflows/development-release.yml"
release_notes="$root/.github/development-release-notes.md"

assert_version() {
  local expected=$1
  shift
  local actual
  actual=$($version_script "$@")
  if [[ $actual != "$expected" ]]; then
    echo "expected $expected, got $actual" >&2
    exit 1
  fi
}

assert_rejected() {
  if $version_script "$@" >/dev/null 2>&1; then
    echo "expected invalid input to be rejected: $*" >&2
    exit 1
  fi
}

assert_version v0.6.2-dev.20260911.42.1.gabcdef0 0.6.1 20260911 42 1 ABCDEF0123456789
assert_version v1.12.100-dev.20260228.7.3.g0123456 1.12.99 20260228 7 3 0123456789abcdef
assert_rejected v0.6.1 20260911 42 1 abcdef0123456789
assert_rejected 0.6.1 20260230 42 1 abcdef0123456789
assert_rejected 0.6.1 20260911 0 1 abcdef0123456789
assert_rejected 0.6.1 20260911 42 1 not-a-sha

required_workflow_text=(
  'run-name: Development build from ${{ github.event_name == '\''push'\'' && github.sha || inputs.ref }}'
  'push:'
  'branches: [main]'
  'workflow_dispatch:'
  'permissions: {}'
  'contents: read'
  'contents: write'
  'path: workflow-tools'
  'ref: ${{ github.workflow_sha }}'
  'gh release create "$tag"'
  'gh api "repos/${GITHUB_REPOSITORY}/git/ref/tags/$tag"'
  '--draft'
  '--prerelease'
  '--latest=false'
  'args: release --skip=publish --clean --config ../workflow-tools/.goreleaser.yaml --release-notes ../workflow-tools/.github/development-release-notes.md'
  'uses: actions/upload-artifact@v7.0.1'
  'uses: actions/download-artifact@v8.0.1'
  'gh release upload "$TAG" -- release-assets/*'
  'gh release edit "$tag" --notes-file "$notes"'
  'gh release edit "$tag" --draft=false --prerelease --latest=false'
  'mise use -g github:dkarter/hwt@${tag#v}'
)

for text in "${required_workflow_text[@]}"; do
  if ! grep -F -- "$text" "$workflow" >/dev/null; then
    echo "development release workflow is missing: $text" >&2
    exit 1
  fi
done

push_checkout=$(sed -n '/      - name: Checkout pushed commit/,/      - name: Checkout requested revision/p' "$workflow")
manual_checkout=$(sed -n '/      - name: Checkout requested revision/,/      - name: Set up Go/p' "$workflow")
build_step=$(sed -n '/      - name: Build release artifacts/,/      - name: Transfer release artifacts/p' "$workflow")
publish_job=$(sed -n '/  publish:/,$p' "$workflow")

for text in \
  'if: ${{ github.event_name == '\''push'\'' }}' \
  'ref: ${{ github.sha }}'; do
  if ! grep -F -- "$text" <<<"$push_checkout" >/dev/null; then
    echo "push checkout is missing: $text" >&2
    exit 1
  fi
done

for text in \
  'if: ${{ github.event_name == '\''workflow_dispatch'\'' }}' \
  'ref: ${{ inputs.ref }}'; do
  if ! grep -F -- "$text" <<<"$manual_checkout" >/dev/null; then
    echo "manual checkout is missing: $text" >&2
    exit 1
  fi
done

if [[ ! -s $release_notes ]]; then
  echo "trusted development release notes are missing or empty: $release_notes" >&2
  exit 1
fi

if grep -E 'GITHUB_TOKEN|GH_TOKEN|github\.token|secrets\.' <<<"$build_step" >/dev/null; then
  echo 'development artifact build must not receive GitHub credentials' >&2
  exit 1
fi

if grep -E '(^|[[:space:]])git([[:space:]]|$)' <<<"$publish_job" >/dev/null; then
  echo 'checkout-free publish job must not invoke git' >&2
  exit 1
fi

echo 'development release checks passed'
