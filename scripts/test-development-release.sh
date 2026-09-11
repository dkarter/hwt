#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version_script="$root/scripts/development-version.sh"
workflow="$root/.github/workflows/development-release.yml"

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
  'workflow_dispatch:'
  'permissions: {}'
  'contents: read'
  'contents: write'
  'gh release create "$tag"'
  '--draft'
  '--prerelease'
  '--latest=false'
  'args: release --skip=publish --clean --config ../workflow-tools/.goreleaser.yaml'
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

echo 'development release checks passed'
